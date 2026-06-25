package task

import (
	"context"
	"slices"
	"strings"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type workloadMoveTargetFunc func(deployv1.Workload) (string, error)

func (s *Service) moveAppWorkloads(ctx context.Context, app *deployv1.App, operation string, workloads *list.List[deployv1.Workload], targetFor workloadMoveTargetFunc) (AppOperationSummary, error) {
	summary := operationSummary(app, operation, workloads.Len())
	plans, targetNode, err := s.planWorkloadMoves(app, workloads, targetFor)
	summary.TargetNode = targetNode
	if err != nil {
		summary.Status = workloadmeta.AssignmentStatusFailed
		return summary, oopsx.B("task").Wrapf(err, "%s target selection failed", operation)
	}
	if len(plans) == 0 {
		summary.Status = "unchanged"
		return summary, nil
	}
	generation := AppGeneration(*app)
	if stopErr := s.stopMovePlans(ctx, app, operation, generation, plans); stopErr != nil {
		summary.Status = workloadmeta.AssignmentStatusFailed
		return summary, stopErr
	}
	moved, err := s.startMovePlans(ctx, app, generation, plans)
	summary.Moved = moved
	if err != nil {
		summary.Status = workloadmeta.AssignmentStatusFailed
		return summary, err
	}
	s.logOperationSummary(summary)
	return summary, nil
}

func operationSummary(app *deployv1.App, operation string, workloadCount int) AppOperationSummary {
	return AppOperationSummary{
		Operation: operation,
		App:       app.Metadata.Name,
		Namespace: workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
		Workloads: workloadCount,
		Status:    workloadmeta.AssignmentStatusRunning,
	}
}

func (s *Service) planWorkloadMoves(app *deployv1.App, workloads *list.List[deployv1.Workload], targetFor workloadMoveTargetFunc) ([]plannedWorkloadMove, string, error) {
	self := s.local.String()
	plans := make([]plannedWorkloadMove, 0, workloads.Len())
	targetNode := ""
	var selectErr error
	workloads.Range(func(_ int, workload deployv1.Workload) bool {
		target, err := normalizeMoveTarget(targetFor, workload)
		if err != nil {
			selectErr = err
			return false
		}
		current, assigned := s.currentWorkloadAssignmentNode(app.Metadata, workload.Name)
		if current == "" {
			current = self
		}
		if assigned && current == target {
			return true
		}
		if targetNode == "" {
			targetNode = target
		}
		plans = append(plans, plannedWorkloadMove{workload: workload, current: current, target: target, assigned: assigned})
		return true
	})
	return plans, targetNode, selectErr
}

func normalizeMoveTarget(targetFor workloadMoveTargetFunc, workload deployv1.Workload) (string, error) {
	target, err := targetFor(workload)
	if err != nil {
		return "", err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", oopsx.B("task").Errorf("empty target node for workload %q", workload.Name)
	}
	return target, nil
}

func (s *Service) stopMovePlans(ctx context.Context, app *deployv1.App, operation, generation string, plans []plannedWorkloadMove) error {
	meta := app.Metadata
	for i := range slices.Backward(plans) {
		plan := &plans[i]
		if s.skipMoveStop(meta, operation, plan) {
			continue
		}
		if err := s.stopWorkload(ctx, meta, plan.workload); err != nil {
			s.applyWorkloadAssignment(ctx, meta, plan.workload, plan.current, workloadmeta.AssignmentStatusFailed, generation, err.Error())
			s.applyWorkloadVolumeBindings(ctx, app, plan.workload, plan.current, volumemeta.BindingStatusFailed, generation, err.Error())
			if operation == OperationFailover {
				s.logger.Warn("failover continuing after source stop failed", "workload", plan.workload.Name, "source_node", plan.current, "error", err)
				continue
			}
			return err
		}
		s.applyWorkloadAssignment(ctx, meta, plan.workload, plan.current, workloadmeta.AssignmentStatusStopped, generation, "")
		s.applyWorkloadVolumeBindings(ctx, app, plan.workload, plan.current, volumemeta.BindingStatusReleased, generation, "")
	}
	return nil
}

func (s *Service) skipMoveStop(meta deployv1.Metadata, operation string, plan *plannedWorkloadMove) bool {
	if !plan.assigned {
		return true
	}
	return operation == OperationFailover &&
		s.workloadAssignmentStatus(meta, plan.workload.Name) == workloadmeta.AssignmentStatusFailed
}

func (s *Service) startMovePlans(ctx context.Context, app *deployv1.App, generation string, plans []plannedWorkloadMove) (int, error) {
	meta := app.Metadata
	moved := 0
	for i := range plans {
		plan := &plans[i]
		s.applyWorkloadAssignment(ctx, meta, plan.workload, plan.target, workloadmeta.AssignmentStatusAssigned, generation, "")
		result, err := s.runWorkloadOnNode(ctx, meta, plan.workload, plan.target)
		if err != nil {
			s.applyWorkloadAssignment(ctx, meta, plan.workload, plan.target, workloadmeta.AssignmentStatusFailed, generation, err.Error())
			s.applyWorkloadVolumeBindings(ctx, app, plan.workload, plan.target, volumemeta.BindingStatusFailed, generation, err.Error())
			return moved, err
		}
		status := strings.TrimSpace(result.Status)
		if status == "" {
			status = workloadmeta.AssignmentStatusRunning
		}
		s.applyWorkloadAssignment(ctx, meta, plan.workload, plan.target, status, generation, "", result.Address)
		s.applyWorkloadVolumeBindings(ctx, app, plan.workload, plan.target, volumemeta.BindingStatusBound, generation, "")
		moved++
	}
	return moved, nil
}

func (s *Service) logOperationSummary(summary AppOperationSummary) {
	s.logger.Info("deploy operation completed",
		"operation", summary.Operation,
		"app", summary.App,
		"namespace", summary.Namespace,
		"workloads", summary.Workloads,
		"moved", summary.Moved,
	)
}
