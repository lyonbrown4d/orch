package task_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/metrics"
	"github.com/lyonbrown4d/orch/internal/nodecapacity"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/observability"
	"github.com/lyonbrown4d/orch/internal/placement"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

const deployReconcileTimeout = 10 * time.Second

type taskHarness struct {
	ctx      context.Context
	cfg      config.Config
	raft     *raftsvc.Service
	store    nodecapacity.SnapshotStore
	catalog  *nodecapacity.Catalog
	runtime  *fakeRuntimeProvider
	registry *registry.Service
	svc      *task.Service
}

type workloadOption func(*deployv1.Workload)

func newTaskHarness(t *testing.T, cfg config.Config, dispatcher task.WorkerDispatcher) *taskHarness {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	local := nodeid.Local{Value: "node-a"}
	raft := raftsvc.New(cfg, logger, local)
	store := raftsvc.NewRaftCapacityStore(raft)
	catalog := nodecapacity.NewCatalog(store)
	fakeRuntime := newFakeRuntimeProvider()
	runtimeManager := orchruntime.NewManager(logger, fakeRuntime)
	registrySvc := registry.NewService(logger)
	svc := task.NewService(logger, newTestMetrics(t, cfg, logger), runtimeManager, registrySvc, cfg, task.Bundle{
		LocalNode:  local,
		Catalog:    catalog,
		Placement:  placement.NewEngine(),
		Raft:       raft,
		Dispatcher: dispatcher,
	})
	return &taskHarness{
		ctx:      t.Context(),
		cfg:      cfg,
		raft:     raft,
		store:    store,
		catalog:  catalog,
		runtime:  fakeRuntime,
		registry: registrySvc,
		svc:      svc,
	}
}

func newTestMetrics(t *testing.T, cfg config.Config, logger *slog.Logger) *metrics.Service {
	t.Helper()
	cfg.Observability.Prometheus.Enabled = false
	cfg.Observability.OTLP.Enabled = false
	obs, err := observability.New(cfg, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	return metrics.New(obs)
}

func dockerWorkload(name, image string, opts ...workloadOption) deployv1.Workload {
	workload := deployv1.Workload{
		Name:    name,
		Kind:    deployv1.WorkloadKindWorker,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: image}},
	}
	for _, opt := range opts {
		opt(&workload)
	}
	return workload
}

func workloadKind(kind deployv1.WorkloadKind) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Kind = kind
	}
}

func workloadResources(cpuMillis, memoryBytes int64) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Resources = &deployv1.Resources{CPUMillis: cpuMillis, MemoryBytes: memoryBytes}
	}
}

func workloadPreferred(nodes ...string) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Scheduling = &deployv1.Scheduling{PreferredNodes: nodes}
	}
}

func workloadDependsOn(names ...string) workloadOption {
	return func(workload *deployv1.Workload) {
		for _, name := range names {
			workload.DependsOn = append(workload.DependsOn, deployv1.WorkloadRef{Name: name})
		}
	}
}

func workloadMount(volume, target string) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Mounts = append(workload.Mounts, deployv1.Mount{
			Volume: deployv1.VolumeRef{Name: volume},
			Target: target,
		})
	}
}

func workloadRollingRollout() workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Rollout = &deployv1.Rollout{Strategy: "rolling", MaxUnavailable: 1}
	}
}

func workloadAutoRollbackRollout() workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Rollout = &deployv1.Rollout{Strategy: "rolling", MaxUnavailable: 1, RollbackOnFailure: true}
	}
}

func workloadProgressDeadline(deadline time.Duration) workloadOption {
	return func(workload *deployv1.Workload) {
		if workload.Rollout == nil {
			workload.Rollout = &deployv1.Rollout{}
		}
		workload.Rollout.ProgressDeadline = deadline.String()
	}
}

func deployApp(name string, workloads ...deployv1.Workload) *deployv1.App {
	return &deployv1.App{
		Metadata:  deployv1.Metadata{Name: name, Namespace: "default"},
		Workloads: workloads,
	}
}

func (h *taskHarness) seedRemoteCapacity(t *testing.T, nodeID string) {
	t.Helper()
	if err := h.store.Upsert(h.ctx, nodecapacity.Snapshot{
		NodeID:           nodeID,
		UpdatedAt:        time.Now(),
		LogicalCPUCores:  8,
		CPUUsagePercent:  5,
		MemoryAvailBytes: 16 << 30,
	}); err != nil {
		t.Fatal(err)
	}
}

func (h *taskHarness) startReconcile() {
	h.svc.StartDeployReconcile(h.ctx)
}

func (h *taskHarness) submitDeploy(t *testing.T, app *deployv1.App) {
	t.Helper()
	if err := h.svc.SubmitDeploy(h.ctx, app); err != nil {
		t.Fatal(err)
	}
}

func (h *taskHarness) applyApp(t *testing.T, app *deployv1.App) {
	t.Helper()
	if err := h.raft.ApplyDeployApp(context.Background(), *app); err != nil {
		t.Fatal(err)
	}
}

func (h *taskHarness) applyWorkerAssignment(t *testing.T, app *deployv1.App, nodeID, status string) {
	t.Helper()
	workloadName := "worker"
	if err := h.raft.ApplyWorkloadAssignment(context.Background(), workloadmeta.Assignment{
		Key:      workloadmeta.AssignmentKey(app.Metadata, workloadName),
		Metadata: app.Metadata,
		Workload: workloadName,
		Node:     nodeID,
		Runtime:  deployv1.RuntimeDocker,
		Status:   status,
	}); err != nil {
		t.Fatal(err)
	}
}

func (h *taskHarness) waitRuntimeDeploy(t *testing.T, timeout time.Duration) deployv1.Workload {
	t.Helper()
	select {
	case got := <-h.runtime.ch:
		return got
	case <-time.After(timeout):
		t.Fatal("timed out waiting for runtime deploy")
		return deployv1.Workload{}
	}
}

func (h *taskHarness) requireNoLocalDeploy(t *testing.T) {
	t.Helper()
	select {
	case got := <-h.runtime.ch:
		t.Fatalf("local runtime should not deploy remote workload, got %q", got.Name)
	default:
	}
}

func (h *taskHarness) requireNoStop(t *testing.T) {
	t.Helper()
	select {
	case stopped := <-h.runtime.stopCh:
		t.Fatalf("unexpected stop for unassigned workload %q", stopped)
	default:
	}
}

func (h *taskHarness) requireRegistryRecord(t *testing.T, name, nodeID, status string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if h.registryHasRecord(name, nodeID, status) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("registry did not converge, got %#v", h.registry.List())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *taskHarness) registryHasRecord(name, nodeID, status string) bool {
	items := h.registry.List()
	got, ok := items.Get(0)
	return items.Len() == 1 && ok && got.Name == name && got.Node == nodeID && got.Status == status
}

func (h *taskHarness) requireAssignment(t *testing.T, app *deployv1.App, workloadName, nodeID, status string) workloadmeta.Assignment {
	t.Helper()
	key := workloadmeta.AssignmentKey(app.Metadata, workloadName)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, ok := h.raft.GetWorkloadAssignment(key)
		if ok && got.Node == nodeID && got.Status == status {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("assignment %q did not converge to node=%q status=%q, got %#v", key, nodeID, status, h.raft.ListWorkloadAssignments())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *taskHarness) requireVolumeBinding(t *testing.T, app *deployv1.App, workloadName, volumeName, nodeID, status string) volumemeta.Binding {
	t.Helper()
	key := volumemeta.BindingKey(app.Metadata, workloadName, volumeName)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, ok := h.raft.GetVolumeBinding(key)
		if ok && got.Node == nodeID && got.Status == status {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("volume binding %q did not converge to node=%q status=%q, got %#v", key, nodeID, status, h.raft.ListVolumeBindings())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
