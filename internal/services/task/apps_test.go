package task_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

type appViewHarness struct {
	raft *raftsvc.Service
	svc  *task.Service
}

func newAppViewHarness() appViewHarness {
	cfg := config.Default()
	logger := slog.New(slog.DiscardHandler)
	raft := raftsvc.New(cfg, logger, nodeid.Local{Value: "node-a"})
	svc := task.NewService(logger, nil, nil, nil, cfg, task.Bundle{Raft: raft})
	return appViewHarness{raft: raft, svc: svc}
}

func TestListAppsAggregatesAssignmentStatus(t *testing.T) {
	harness := newAppViewHarness()
	app := appViewDemo("demo", appViewDockerWorkload("api", "api"), appViewDockerWorkload("worker", "worker"))
	harness.applyApp(t, app)
	harness.requirePendingApp(t, app.Metadata, 2)

	now := time.Now().UTC()
	generation := task.AppGeneration(app)
	harness.applyAssignment(t, app, "api", "api", workloadmeta.AssignmentStatusRunning, generation, now)
	harness.applyAssignment(t, app, "worker", "worker", workloadmeta.AssignmentStatusStopped, generation, now.Add(time.Second))
	harness.requirePartialApp(t, app.Metadata, now.Add(time.Second))

	harness.applyAssignment(t, app, "worker", "worker", workloadmeta.AssignmentStatusRunning, generation, now.Add(2*time.Second))
	harness.requireRunningList(t)
}

func TestListAppsTreatsStaleAssignmentsAsPending(t *testing.T) {
	harness := newAppViewHarness()
	app := appViewDemo("demo", appViewDockerWorkload("api", "api:v2"))
	harness.applyApp(t, app)
	harness.applyAssignment(t, app, "api", "api:v1", workloadmeta.AssignmentStatusRunning, "old", time.Now().UTC())

	view := harness.requireApp(t, app.Metadata)
	requirePendingView(t, view, 1)
	workload := requireFirstWorkload(t, view)
	if workload.Status != task.AppStatusPending || workload.Generation != "old" {
		t.Fatalf("stale workload = %#v", workload)
	}
}

func TestListAppsTreatsMissingGenerationAsPending(t *testing.T) {
	harness := newAppViewHarness()
	app := appViewDemo("demo", appViewDockerWorkload("api", "api:v1"))
	harness.applyApp(t, app)
	harness.applyAssignment(t, app, "api", "api:v1", workloadmeta.AssignmentStatusRunning, "", time.Now().UTC())

	view := harness.requireApp(t, app.Metadata)
	requirePendingView(t, view, 1)
	workload := requireFirstWorkload(t, view)
	if workload.Status != task.AppStatusPending || workload.Generation != "" {
		t.Fatalf("missing-generation workload = %#v", workload)
	}
}

func TestListAppsIncludesRevisionSummary(t *testing.T) {
	harness := newAppViewHarness()
	appV1 := appViewDemo("demo", appViewDockerWorkload("api", "api:v1"))
	appV2 := appViewDemo("demo", appViewDockerWorkload("api", "api:v2"))
	harness.applyApp(t, appV1)
	harness.applyApp(t, appV2)

	view := harness.requireApp(t, appV2.Metadata)
	if view.RevisionCount != 1 || !view.RollbackAvailable || view.PreviousGeneration != task.AppGeneration(appV1) {
		t.Fatalf("revision summary = count:%d rollback:%t previous:%q want previous %q", view.RevisionCount, view.RollbackAvailable, view.PreviousGeneration, task.AppGeneration(appV1))
	}
}

func appViewDemo(name string, workloads ...deployv1.Workload) deployv1.App {
	return deployv1.App{
		Metadata:  deployv1.Metadata{Name: name, Namespace: "default"},
		Workloads: workloads,
	}
}

func appViewDockerWorkload(name, image string) deployv1.Workload {
	return deployv1.Workload{
		Name:    name,
		Kind:    deployv1.WorkloadKindService,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: image}},
	}
}

func (h appViewHarness) applyApp(t *testing.T, app deployv1.App) {
	t.Helper()
	if err := h.raft.ApplyDeployApp(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func (h appViewHarness) applyAssignment(t *testing.T, app deployv1.App, workloadName, artifact, status, generation string, updatedAt time.Time) {
	t.Helper()
	if err := h.raft.ApplyWorkloadAssignment(context.Background(), workloadmeta.Assignment{
		Key:        workloadmeta.AssignmentKey(app.Metadata, workloadName),
		Metadata:   app.Metadata,
		Workload:   workloadName,
		Node:       "node-a",
		Runtime:    deployv1.RuntimeDocker,
		Artifact:   artifact,
		Status:     status,
		Generation: generation,
		UpdatedAt:  updatedAt,
	}); err != nil {
		t.Fatal(err)
	}
}

func (h appViewHarness) requireApp(t *testing.T, meta deployv1.Metadata) task.AppView {
	t.Helper()
	view, ok := h.svc.GetApp(meta)
	if !ok {
		t.Fatal("expected app")
	}
	return view
}

func (h appViewHarness) requirePendingApp(t *testing.T, meta deployv1.Metadata, pendingCount int) {
	t.Helper()
	view := h.requireApp(t, meta)
	requirePendingView(t, view, pendingCount)
	if view.DesiredGeneration == "" || view.ObservedGeneration != "" {
		t.Fatalf("pending generations = desired:%q observed:%q", view.DesiredGeneration, view.ObservedGeneration)
	}
}

func (h appViewHarness) requirePartialApp(t *testing.T, meta deployv1.Metadata, wantTransition time.Time) {
	t.Helper()
	view := h.requireApp(t, meta)
	if view.Status != task.AppStatusPartial || view.Running != 1 || view.Stopped != 1 || !view.LastTransitionAt.Equal(wantTransition) {
		t.Fatalf("partial app = %#v", view)
	}
	if view.ObservedGeneration != view.DesiredGeneration {
		t.Fatalf("partial generations = desired:%q observed:%q", view.DesiredGeneration, view.ObservedGeneration)
	}
}

func (h appViewHarness) requireRunningList(t *testing.T) {
	t.Helper()
	apps := h.svc.ListApps()
	if apps.Len() != 1 {
		t.Fatalf("apps len = %d", apps.Len())
	}
	running, _ := apps.Get(0)
	if running.Status != task.AppStatusRunning || running.Running != 2 {
		t.Fatalf("running app = %#v", running)
	}
}

func requirePendingView(t *testing.T, view task.AppView, pendingCount int) {
	t.Helper()
	if view.Status != task.AppStatusPending || view.Pending != pendingCount || view.ObservedGeneration != "" {
		t.Fatalf("pending app = %#v", view)
	}
}

func requireFirstWorkload(t *testing.T, view task.AppView) task.AppWorkloadView {
	t.Helper()
	workload, ok := view.Workloads.Get(0)
	if !ok {
		t.Fatal("expected first workload")
	}
	return workload
}
