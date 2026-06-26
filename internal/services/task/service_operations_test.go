package task_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workerapi"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestSubmitMigrateMovesWorkloadToTargetNode(t *testing.T) {
	t.Parallel()

	dispatchCh := make(chan workerapi.DeployWorkloadBody, 1)
	worker := newDeployWorkerServer(t, dispatchCh, workloadmeta.AssignmentStatusRunning)
	cfg := config.Default()
	cfg.Cluster.Nodes = map[string]string{"node-b": worker.URL}
	harness := newTaskHarness(t, cfg, task.NewHTTPWorkerDispatcher(cfg))
	app := deployApp("migrate-demo", dockerWorkload("worker", "busybox"))
	harness.applyApp(t, app)
	harness.applyWorkerAssignment(t, app, "node-a", workloadmeta.AssignmentStatusRunning)

	summary, err := harness.svc.SubmitMigrate(context.Background(), app.Metadata, task.AppOperationOptions{TargetNode: "node-b"})
	if err != nil {
		t.Fatal(err)
	}
	requireMoveSummary(t, summary, 1, "node-b")
	requireWorkerDispatch(t, waitWorkerDispatch(t, dispatchCh), "node-b")
	harness.requireAssignment(t, app, "worker", "node-b", workloadmeta.AssignmentStatusRunning)
}

func TestSubmitFailoverMovesFailedWorkloadToLocalNode(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("failover-demo", dockerWorkload("worker", "busybox"))
	harness.applyApp(t, app)
	harness.applyWorkerAssignment(t, app, "node-b", workloadmeta.AssignmentStatusFailed)

	summary, err := harness.svc.SubmitFailover(context.Background(), app.Metadata, task.AppOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	requireMoveSummary(t, summary, 1, "")
	requireWorkloadName(t, harness.waitRuntimeDeploy(t, 3*time.Second), "worker")
	harness.requireAssignment(t, app, "worker", "node-a", workloadmeta.AssignmentStatusRunning)
}

func TestSubmitRebalanceStartsUnassignedWorkloadWithoutStop(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("rebalance-demo", dockerWorkload("worker", "busybox"))
	harness.applyApp(t, app)

	summary, err := harness.svc.SubmitRebalance(context.Background(), app.Metadata, task.AppOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	requireRunningMoveSummary(t, summary, 1)
	harness.requireNoStop(t)
	requireWorkloadName(t, harness.waitRuntimeDeploy(t, 3*time.Second), "worker")
	harness.requireAssignment(t, app, "worker", "node-a", workloadmeta.AssignmentStatusRunning)
}

func TestSubmitFailoverContinuesWhenSourceStopFails(t *testing.T) {
	t.Parallel()

	dispatcher := &failoverStopFailDispatcher{deployCh: make(chan workerapi.DeployWorkloadBody, 1)}
	harness := newTaskHarness(t, config.Default(), dispatcher)
	app := deployApp("failover-stop-fails", dockerWorkload("worker", "busybox"))
	harness.applyApp(t, app)
	harness.applyWorkerAssignment(t, app, "node-b", workloadmeta.AssignmentStatusRunning)

	summary, err := harness.svc.SubmitFailover(context.Background(), app.Metadata, task.AppOperationOptions{
		TargetNode: "node-c",
		Workloads:  []string{"worker"},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMoveSummary(t, summary, 1, "node-c")
	requireWorkerDispatch(t, waitWorkerDispatch(t, dispatcher.deployCh), "node-c")
	harness.requireAssignment(t, app, "worker", "node-c", workloadmeta.AssignmentStatusRunning)
}

type failoverStopFailDispatcher struct {
	deployCh chan workerapi.DeployWorkloadBody
}

func (d *failoverStopFailDispatcher) DispatchWorkload(_ context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (task.DispatchResult, error) {
	d.deployCh <- workerapi.DeployWorkloadBody{Metadata: meta, Workload: workload, Node: nodeID}
	return task.DispatchResult{Accepted: true, Node: nodeID, Status: workloadmeta.AssignmentStatusRunning, Workload: workload.Name}, nil
}

func (d *failoverStopFailDispatcher) StopWorkload(context.Context, string, deployv1.Metadata, deployv1.Workload) (task.DispatchResult, error) {
	return task.DispatchResult{}, errors.New("worker unavailable")
}

func (d *failoverStopFailDispatcher) WorkloadStatus(context.Context, string, deployv1.Metadata, deployv1.Workload) (orchruntime.Status, error) {
	return orchruntime.Status{}, errors.New("not implemented")
}

func (d *failoverStopFailDispatcher) WorkloadLogs(context.Context, string, deployv1.Metadata, deployv1.Workload, orchruntime.LogOptions) (orchruntime.LogResult, error) {
	return orchruntime.LogResult{}, errors.New("not implemented")
}
func requireMoveSummary(t *testing.T, summary task.AppOperationSummary, moved int, targetNode string) {
	t.Helper()
	if summary.Moved != moved {
		t.Fatalf("summary = %#v", summary)
	}
	if targetNode != "" && summary.TargetNode != targetNode {
		t.Fatalf("summary = %#v", summary)
	}
}

func requireRunningMoveSummary(t *testing.T, summary task.AppOperationSummary, moved int) {
	t.Helper()
	if summary.Moved != moved || summary.Status != workloadmeta.AssignmentStatusRunning {
		t.Fatalf("summary = %#v", summary)
	}
}
