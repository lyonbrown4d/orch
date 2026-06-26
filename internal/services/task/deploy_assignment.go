package task

import (
	"context"
	"strings"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) workloadAssignment(meta deployv1.Metadata, workloadName string) (workloadmeta.Assignment, bool) {
	if s == nil || s.raft == nil {
		return workloadmeta.Assignment{}, false
	}
	return s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workloadName))
}

func workloadAssignmentConverged(assignment workloadmeta.Assignment, generation string) bool {
	return strings.TrimSpace(assignment.Generation) == strings.TrimSpace(generation) && assignment.Status == workloadmeta.AssignmentStatusRunning
}

func (s *Service) handleExistingAssignment(
	ctx context.Context,
	app *deployv1.App,
	workload deployv1.Workload,
	assignment workloadmeta.Assignment,
	generation string,
	opts deployOptions,
) (bool, error) {
	if workloadAssignmentConverged(assignment, generation) {
		return true, nil
	}
	if shouldSkipStoppedAssignment(assignment, generation, opts) {
		return true, nil
	}
	if !s.canRetryFailedAssignment(workload, assignment, generation) {
		return true, nil
	}
	if failedAssignmentSameGeneration(assignment, generation) && !s.waitWorkloadRestartDelay(ctx, workload) {
		return true, oopsx.B("task").Wrapf(ctx.Err(), "wait restart delay for workload %s", workload.Name)
	}
	if err := s.stopPreviousGeneration(ctx, app.Metadata, workload, assignment, generation); err != nil {
		s.recordDeployFailure(ctx, app, workload, strings.TrimSpace(assignment.Node), generation, err)
		return true, err
	}
	return false, nil
}

func shouldSkipStoppedAssignment(assignment workloadmeta.Assignment, generation string, opts deployOptions) bool {
	return !opts.forceStopped && strings.TrimSpace(assignment.Generation) == strings.TrimSpace(generation) && strings.TrimSpace(assignment.Status) == workloadmeta.AssignmentStatusStopped
}

func (s *Service) canRetryFailedAssignment(workload deployv1.Workload, assignment workloadmeta.Assignment, generation string) bool {
	if !failedAssignmentSameGeneration(assignment, generation) {
		return true
	}
	return s.allowWorkloadRestart(workload, assignment.Key, generation)
}

func failedAssignmentSameGeneration(assignment workloadmeta.Assignment, generation string) bool {
	return strings.TrimSpace(assignment.Generation) == strings.TrimSpace(generation) && strings.TrimSpace(assignment.Status) == workloadmeta.AssignmentStatusFailed
}

func (s *Service) stopPreviousGeneration(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, assignment workloadmeta.Assignment, generation string) error {
	if strings.TrimSpace(assignment.Generation) == strings.TrimSpace(generation) {
		return nil
	}
	nodeID := strings.TrimSpace(assignment.Node)
	if nodeID == "" || !assignmentMayHaveRuntime(assignment.Status) {
		return nil
	}
	s.logger.Info("rollout stopping previous workload generation",
		"app", meta.Name,
		"workload", workload.Name,
		"node", nodeID,
		"previous_generation", strings.TrimSpace(assignment.Generation),
		"next_generation", strings.TrimSpace(generation),
	)
	return s.stopWorkloadOnNode(ctx, meta, workload, nodeID)
}

func assignmentMayHaveRuntime(status string) bool {
	switch strings.TrimSpace(status) {
	case workloadmeta.AssignmentStatusAssigned, workloadmeta.AssignmentStatusRunning:
		return true
	default:
		return false
	}
}
