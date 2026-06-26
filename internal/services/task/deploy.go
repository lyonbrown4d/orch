package task

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// SubmitDeploy validates the app and appends it to the replicated desired state.
// Local container startup happens asynchronously via [Service.StartDeployReconcile] on each node.
func (s *Service) SubmitDeploy(ctx context.Context, app *deployv1.App) error {
	if app == nil {
		return oopsx.B("task").Errorf("nil app")
	}
	if err := app.Validate(); err != nil {
		s.metrics.IncDeployApp(ctx, "invalid")
		return oopsx.B("task").Wrapf(err, "validate app")
	}
	if s.raft == nil {
		return oopsx.B("task").Errorf("raft service unavailable")
	}
	if err := s.raft.ApplyDeployApp(ctx, *app); err != nil {
		return oopsx.B("task").Wrapf(err, "replicate deploy")
	}
	s.logger.Info("deploy submitted", "app", app.Metadata.Name, "workloads", len(app.Workloads))
	return nil
}

// DeployApp is an alias for [Service.SubmitDeploy] (HTTP handlers and older callers).
func (s *Service) DeployApp(ctx context.Context, app *deployv1.App) error {
	return s.SubmitDeploy(ctx, app)
}

// deployAppWorkloads runs placement against the node resource catalog, then deploys locally or dispatches to a
// configured worker API when placement selects a remote node.
func (s *Service) deployAppWorkloads(ctx context.Context, app *deployv1.App) error {
	workloads, generation, err := s.prepareDeploy(ctx, app)
	if err != nil {
		return err
	}
	if err := s.deployOrderedWorkloads(ctx, app, workloads, generation); err != nil {
		return err
	}
	s.metrics.IncDeployApp(ctx, "success")
	s.logger.Info("application deployed", "app", app.Metadata.Name, "workloads", len(app.Workloads))
	return nil
}

func (s *Service) prepareDeploy(ctx context.Context, app *deployv1.App) (*list.List[deployv1.Workload], string, error) {
	if err := app.Validate(); err != nil {
		s.metrics.IncDeployApp(ctx, "invalid")
		return nil, "", oopsx.B("task").Wrapf(err, "validate app")
	}
	if err := s.catalog.RefreshLocal(ctx, s.local, s.cfg); err != nil {
		s.logger.Warn("refresh local node capacity before placement", "error", err)
	}
	workloads, err := app.WorkloadsInDependencyOrder()
	if err != nil {
		s.metrics.IncDeployApp(ctx, "invalid")
		return nil, "", oopsx.B("task").Wrapf(err, "order workloads")
	}
	return workloads, AppGeneration(*app), nil
}

func (s *Service) deployOrderedWorkloads(ctx context.Context, app *deployv1.App, workloads *list.List[deployv1.Workload], generation string) error {
	self := s.local.String()
	var deployErr error
	workloads.Range(func(_ int, workload deployv1.Workload) bool {
		if err := s.deployOneWorkload(ctx, app, workload, self, generation); err != nil {
			deployErr = err
			return false
		}
		return true
	})
	return deployErr
}

func (s *Service) deployOneWorkload(ctx context.Context, app *deployv1.App, workload deployv1.Workload, self, generation string) error {
	meta := app.Metadata
	if s.workloadConverged(meta, workload, generation) {
		return nil
	}
	chosen, err := s.chooseWorkloadNode(ctx, workload, self)
	if err != nil {
		s.recordDeployFailure(ctx, app, workload, "", generation, err)
		return oopsx.B("task").Wrapf(err, "placement workload %s", workload.Name)
	}
	s.applyWorkloadAssignment(ctx, meta, workload, chosen, workloadmeta.AssignmentStatusAssigned, generation, "")
	if chosen != self {
		return s.deployRemoteWorkload(ctx, app, workload, chosen, generation)
	}
	return s.deployLocalAssignedWorkload(ctx, app, workload, chosen, generation)
}

func (s *Service) workloadConverged(meta deployv1.Metadata, workload deployv1.Workload, generation string) bool {
	if s == nil || s.raft == nil {
		return false
	}
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workload.Name))
	if !ok {
		return false
	}
	return strings.TrimSpace(assignment.Generation) == strings.TrimSpace(generation) && assignment.Status == workloadmeta.AssignmentStatusRunning
}

func (s *Service) deployRemoteWorkload(ctx context.Context, app *deployv1.App, workload deployv1.Workload, nodeID, generation string) error {
	meta := app.Metadata
	result, err := s.dispatchWorkload(ctx, meta, workload, nodeID)
	if err != nil {
		s.recordDeployFailure(ctx, app, workload, nodeID, generation, err)
		return err
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "dispatched"
	}
	s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), status)
	s.registry.Upsert(registry.WorkloadRecord{
		Name:     workload.Name,
		Node:     nodeID,
		Runtime:  string(workload.Runtime),
		Artifact: runconfig.ArtifactSummary(workload.Run),
		Status:   status,
	})
	s.applyWorkloadAssignment(ctx, meta, workload, nodeID, status, generation, "", result.Address)
	s.applyWorkloadVolumeBindings(ctx, app, workload, nodeID, volumemeta.BindingStatusBound, generation, "")
	return nil
}
func (s *Service) deployLocalAssignedWorkload(ctx context.Context, app *deployv1.App, workload deployv1.Workload, nodeID, generation string) error {
	meta := app.Metadata
	result, err := s.runLocalWorkload(ctx, meta, workload, nodeID)
	if err != nil {
		s.recordDeployFailure(ctx, app, workload, nodeID, generation, err)
		return err
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = workloadmeta.AssignmentStatusRunning
	}
	s.applyWorkloadAssignment(ctx, meta, workload, nodeID, status, generation, "", result.Address)
	s.applyWorkloadVolumeBindings(ctx, app, workload, nodeID, volumemeta.BindingStatusBound, generation, "")
	s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "success")
	return nil
}
func (s *Service) recordDeployFailure(ctx context.Context, app *deployv1.App, workload deployv1.Workload, nodeID, generation string, err error) {
	meta := app.Metadata
	if s.skipStaleDeployFailure(meta, workload.Name, nodeID, generation) {
		s.logger.Warn("skip stale deploy failure after workload converged elsewhere", "workload", workload.Name, "failed_node", nodeID, "error", err)
		return
	}
	s.applyWorkloadAssignment(ctx, meta, workload, nodeID, workloadmeta.AssignmentStatusFailed, generation, err.Error())
	s.applyWorkloadVolumeBindings(ctx, app, workload, nodeID, volumemeta.BindingStatusFailed, generation, err.Error())
	s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "failed")
	s.metrics.IncDeployApp(ctx, "failed")
}

func (s *Service) skipStaleDeployFailure(meta deployv1.Metadata, workloadName, failedNodeID, generation string) bool {
	if s == nil || s.raft == nil {
		return false
	}
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workloadName))
	if !ok {
		return false
	}
	return assignmentConvergedOnDifferentNode(assignment, failedNodeID, generation)
}

func assignmentConvergedOnDifferentNode(assignment workloadmeta.Assignment, failedNodeID, generation string) bool {
	if strings.TrimSpace(assignment.Generation) != strings.TrimSpace(generation) {
		return false
	}
	if !assignmentConvergedStatus(assignment.Status) {
		return false
	}
	currentNode := strings.TrimSpace(assignment.Node)
	failedNodeID = strings.TrimSpace(failedNodeID)
	return currentNode != "" && failedNodeID != "" && currentNode != failedNodeID
}

func assignmentConvergedStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case workloadmeta.AssignmentStatusAssigned, workloadmeta.AssignmentStatusRunning:
		return true
	default:
		return false
	}
}

func (s *Service) chooseWorkloadNode(ctx context.Context, workload deployv1.Workload, self string) (string, error) {
	chosen, err := s.placement.Choose(ctx, workload, s.catalog, self)
	if err == nil {
		return chosen, nil
	}
	fallback, ok := s.preferredConfiguredNode(workload, self)
	if !ok {
		return "", oopsx.B("task").Wrapf(err, "choose workload node")
	}
	s.logger.Warn("placement fallback to configured preferred node without capacity snapshot",
		"node", fallback,
		"workload", workload.Name,
		"error", err,
	)
	return fallback, nil
}

func (s *Service) preferredConfiguredNode(workload deployv1.Workload, self string) (string, bool) {
	if s == nil || workload.Scheduling == nil || len(workload.Scheduling.PreferredNodes) == 0 {
		return "", false
	}
	for _, raw := range workload.Scheduling.PreferredNodes {
		nodeID := strings.TrimSpace(raw)
		if nodeID == "" || nodeID == strings.TrimSpace(self) {
			continue
		}
		if _, ok := s.cfg.Cluster.NodeURL(nodeID); ok {
			return nodeID, true
		}
	}
	return "", false
}

func assignmentNodeOrEmpty(raft *raftsvc.Service, meta deployv1.Metadata, workloadName string) string {
	if raft == nil {
		return ""
	}
	assignment, ok := raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workloadName))
	if !ok {
		return ""
	}
	return assignment.Node
}
