package task

import (
	"context"
	"slices"
	"strings"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) SubmitDelete(ctx context.Context, meta deployv1.Metadata) error {
	meta = normalizeOperationMetadata(meta)
	if meta.Name == "" {
		return oopsx.B("task").Errorf("metadata.name is required")
	}
	if s.raft == nil {
		return oopsx.B("task").Errorf("raft service unavailable")
	}
	app, ok := s.raft.GetDesiredDeployApp(meta)
	if err := s.raft.ApplyDeleteDeployApp(ctx, meta); err != nil {
		return oopsx.B("task").Wrapf(err, "delete desired app")
	}
	if ok {
		if err := s.stopAppWorkloads(ctx, &app); err != nil {
			return err
		}
	}
	s.logger.Info("deploy deleted", "app", meta.Name, "namespace", workloadmeta.NamespaceOrDefault(meta.Namespace))
	return nil
}

func (s *Service) SubmitStop(ctx context.Context, meta deployv1.Metadata) error {
	return s.submitExistingDeployApp(ctx, meta, "stopped", s.stopAppWorkloads)
}

func (s *Service) SubmitStart(ctx context.Context, meta deployv1.Metadata) error {
	return s.submitExistingDeployApp(ctx, meta, "started", s.deployAppWorkloads)
}

func (s *Service) submitExistingDeployApp(ctx context.Context, meta deployv1.Metadata, action string, run func(context.Context, *deployv1.App) error) error {
	meta = normalizeOperationMetadata(meta)
	if meta.Name == "" {
		return oopsx.B("task").Errorf("metadata.name is required")
	}
	if s.raft == nil {
		return oopsx.B("task").Errorf("raft service unavailable")
	}
	app, ok := s.raft.GetDesiredDeployApp(meta)
	if !ok {
		return oopsx.B("task").Errorf("deploy app %s/%s not found", workloadmeta.NamespaceOrDefault(meta.Namespace), meta.Name)
	}
	if err := run(ctx, &app); err != nil {
		return err
	}
	s.logger.Info("deploy "+action, "app", meta.Name, "namespace", workloadmeta.NamespaceOrDefault(meta.Namespace))
	return nil
}

func (s *Service) SubmitRestart(ctx context.Context, meta deployv1.Metadata) error {
	if err := s.SubmitStop(ctx, meta); err != nil {
		return err
	}
	if err := s.SubmitStart(ctx, meta); err != nil {
		return err
	}
	s.logger.Info("deploy restarted", "app", meta.Name, "namespace", workloadmeta.NamespaceOrDefault(meta.Namespace))
	return nil
}

func normalizeOperationMetadata(meta deployv1.Metadata) deployv1.Metadata {
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Namespace = strings.TrimSpace(meta.Namespace)
	return meta
}

func (s *Service) stopAppWorkloads(ctx context.Context, app *deployv1.App) error {
	if app == nil {
		return nil
	}
	workloads, err := app.WorkloadsInDependencyOrder()
	if err != nil {
		return oopsx.B("task").Wrapf(err, "order workloads")
	}
	generation := AppGeneration(*app)
	values := workloads.Values()
	for i := range slices.Backward(values) {
		workload := values[i]
		if err := s.stopWorkload(ctx, app.Metadata, workload); err != nil {
			nodeID := assignmentNodeOrEmpty(s.raft, app.Metadata, workload.Name)
			s.applyWorkloadAssignment(ctx, app.Metadata, workload, nodeID, workloadmeta.AssignmentStatusFailed, generation, err.Error())
			s.applyWorkloadVolumeBindings(ctx, app, workload, nodeID, volumemeta.BindingStatusFailed, generation, err.Error())
			return err
		}
		nodeID := assignmentNodeOrEmpty(s.raft, app.Metadata, workload.Name)
		s.applyWorkloadAssignment(ctx, app.Metadata, workload, nodeID, workloadmeta.AssignmentStatusStopped, generation, "")
		s.applyWorkloadVolumeBindings(ctx, app, workload, nodeID, volumemeta.BindingStatusReleased, generation, "")
	}
	return nil
}

func (s *Service) stopWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload) error {
	nodeID := s.stopTargetNode(meta, workload.Name)
	if nodeID != "" && nodeID != s.local.String() {
		return s.stopRemoteWorkload(ctx, meta, workload, nodeID)
	}
	return s.stopLocalWorkload(ctx, meta, workload)
}

func (s *Service) stopTargetNode(meta deployv1.Metadata, workloadName string) string {
	key := workloadmeta.AssignmentKey(meta, workloadName)
	assignment, ok := s.raft.GetWorkloadAssignment(key)
	if ok && strings.TrimSpace(assignment.Node) != "" {
		return strings.TrimSpace(assignment.Node)
	}
	return s.local.String()
}

func (s *Service) stopRemoteWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) error {
	if s.dispatcher == nil {
		return oopsx.B("task").Errorf("workload %q assigned to remote node %q but worker dispatcher is unavailable", workload.Name, nodeID)
	}
	result, err := s.dispatcher.StopWorkload(ctx, nodeID, meta, workload)
	if err != nil {
		return oopsx.B("task").Wrapf(err, "stop workload %s on node %s", workload.Name, nodeID)
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = workloadmeta.AssignmentStatusStopped
	}
	s.registry.Delete(workload.Name)
	s.logger.Info("workload stop dispatched", "workload", workload.Name, "node", nodeID, "runtime", workload.Runtime, "status", status)
	return nil
}

func (s *Service) stopLocalWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload) error {
	if err := s.runtime.Stop(ctx, workload.Runtime, meta, workload.Name); err != nil {
		return oopsx.B("task").Wrapf(err, "stop workload %s", workload.Name)
	}
	s.registry.Delete(workload.Name)
	return nil
}

// WorkerDeployResult describes the runtime address returned by a worker deploy.
type WorkerDeployResult struct {
	Address string
}

// DeployWorkerWorkload executes a workload assigned by another node. It intentionally bypasses Raft desired-state
// mutation; callers must already have gone through SubmitDeploy on the scheduling node.
func (s *Service) DeployWorkerWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, assignedNode string) (WorkerDeployResult, error) {
	if err := s.validateWorkerWorkload(meta, workload, assignedNode); err != nil {
		return WorkerDeployResult{}, err
	}
	self := s.local.String()
	result, err := s.runLocalWorkload(ctx, meta, workload, self)
	if err != nil {
		s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "failed")
		return WorkerDeployResult{}, err
	}
	s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "success")
	return WorkerDeployResult{Address: result.Address}, nil
}
func (s *Service) StopWorkerWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, assignedNode string) error {
	if err := s.validateWorkerWorkload(meta, workload, assignedNode); err != nil {
		return err
	}
	if err := s.runtime.Stop(ctx, workload.Runtime, meta, workload.Name); err != nil {
		s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "failed")
		return oopsx.B("task", "worker").Wrapf(err, "stop worker workload")
	}
	s.registry.Delete(workload.Name)
	s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), workloadmeta.AssignmentStatusStopped)
	return nil
}

func (s *Service) WorkerWorkloadRuntimeStatus(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, assignedNode string) (runtime.Status, error) {
	if err := s.validateWorkerWorkload(meta, workload, assignedNode); err != nil {
		return runtime.Status{}, err
	}
	if s.runtime == nil {
		return runtime.Status{}, oopsx.B("task", "worker").Errorf("runtime manager unavailable")
	}
	out, err := s.runtime.Status(ctx, workload.Runtime, meta, workload.Name)
	if err != nil {
		return runtime.Status{}, oopsx.B("task", "worker").Wrapf(err, "read worker workload runtime status")
	}
	return out, nil
}

func (s *Service) WorkerWorkloadRuntimeLogs(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, assignedNode string, opts runtime.LogOptions) (runtime.LogResult, error) {
	if err := s.validateWorkerWorkload(meta, workload, assignedNode); err != nil {
		return runtime.LogResult{}, err
	}
	if s.runtime == nil {
		return runtime.LogResult{}, oopsx.B("task", "worker").Errorf("runtime manager unavailable")
	}
	out, err := s.runtime.Logs(ctx, workload.Runtime, meta, workload.Name, opts)
	if err != nil {
		return runtime.LogResult{}, oopsx.B("task", "worker").Wrapf(err, "read worker workload runtime logs")
	}
	return out, nil
}

func (s *Service) validateWorkerWorkload(meta deployv1.Metadata, workload deployv1.Workload, assignedNode string) error {
	if strings.TrimSpace(meta.Name) == "" {
		return oopsx.B("task", "worker").Errorf("metadata.name is required")
	}
	if strings.TrimSpace(workload.Name) == "" {
		return oopsx.B("task", "worker").Errorf("workload.name is required")
	}
	self := s.local.String()
	if nodeID := strings.TrimSpace(assignedNode); nodeID != "" && self != "" && nodeID != self {
		return oopsx.B("task", "worker").Errorf("workload %q assigned to node %q, local node is %q", workload.Name, nodeID, self)
	}
	return nil
}
