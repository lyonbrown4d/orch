package task_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestSubmitStartRollsOutGenerationChangeWithStopThenStart(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	appV1 := deployApp("rollout-demo", dockerWorkload("web", "nginx:1",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	harness.applyApp(t, appV1)
	if err := harness.svc.SubmitStart(harness.ctx, appV1.Metadata); err != nil {
		t.Fatal(err)
	}
	harness.requireAssignment(t, appV1, "web", "node-a", workloadmeta.AssignmentStatusRunning)

	appV2 := deployApp("rollout-demo", dockerWorkload("web", "nginx:2",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	harness.applyApp(t, appV2)
	if err := harness.svc.SubmitStart(harness.ctx, appV2.Metadata); err != nil {
		t.Fatal(err)
	}

	requireRuntimeEvents(t, harness.runtime.runtimeEvents(), []string{
		"deploy:web:nginx:1",
		"stop:web",
		"deploy:web:nginx:2",
	})
	assignment := harness.requireAssignment(t, appV2, "web", "node-a", workloadmeta.AssignmentStatusRunning)
	if assignment.Generation != task.AppGeneration(*appV2) {
		t.Fatalf("assignment generation = %q, want %q", assignment.Generation, task.AppGeneration(*appV2))
	}
	requireAssignmentPayload(t, assignment, deployv1.RuntimeDocker, "nginx:2")
}

func TestSubmitStartMarksRolloutFailedWhenReplacementStartFails(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	appV1 := deployApp("rollout-fail", dockerWorkload("web", "nginx:1",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	harness.applyApp(t, appV1)
	if err := harness.svc.SubmitStart(harness.ctx, appV1.Metadata); err != nil {
		t.Fatal(err)
	}

	appV2 := deployApp("rollout-fail", dockerWorkload("web", "nginx:2",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	harness.runtime.failDeployArtifact("nginx:2", errors.New("replacement failed"))
	harness.applyApp(t, appV2)
	if err := harness.svc.SubmitStart(harness.ctx, appV2.Metadata); err == nil {
		t.Fatal("expected rollout replacement failure")
	}

	requireRuntimeEvents(t, harness.runtime.runtimeEvents(), []string{
		"deploy:web:nginx:1",
		"stop:web",
		"deploy:web:nginx:2",
	})
	assignment := harness.requireAssignment(t, appV2, "web", "node-a", workloadmeta.AssignmentStatusFailed)
	if assignment.Generation != task.AppGeneration(*appV2) {
		t.Fatalf("assignment generation = %q, want %q", assignment.Generation, task.AppGeneration(*appV2))
	}
	requireAssignmentError(t, assignment)
}

func TestSubmitStartAutoRollsBackWhenReplacementStartFails(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	appV1 := deployApp("rollout-auto", dockerWorkload("web", "nginx:1",
		workloadKind(deployv1.WorkloadKindService),
		workloadAutoRollbackRollout(),
	))
	appV2 := deployApp("rollout-auto", dockerWorkload("web", "nginx:2",
		workloadKind(deployv1.WorkloadKindService),
		workloadAutoRollbackRollout(),
	))
	harness.submitDeploy(t, appV1)
	if err := harness.svc.SubmitStart(harness.ctx, appV1.Metadata); err != nil {
		t.Fatal(err)
	}

	harness.runtime.failDeployArtifact("nginx:2", errors.New("replacement failed"))
	harness.submitDeploy(t, appV2)
	if err := harness.svc.SubmitStart(harness.ctx, appV2.Metadata); err == nil {
		t.Fatal("expected rollout replacement failure")
	}

	got, ok := harness.raft.GetDesiredDeployApp(appV1.Metadata)
	if !ok {
		t.Fatal("desired app missing after auto rollback")
	}
	if got.Workloads[0].Run.Artifact.Image != "nginx:1" {
		t.Fatalf("desired image after auto rollback = %q", got.Workloads[0].Run.Artifact.Image)
	}
	previous, ok := harness.raft.GetPreviousDeployApp(appV1.Metadata)
	if !ok || previous.App.Workloads[0].Run.Artifact.Image != "nginx:2" {
		t.Fatalf("previous after auto rollback = %#v ok=%t", previous, ok)
	}
	assignment := harness.requireAssignment(t, appV2, "web", "node-a", workloadmeta.AssignmentStatusFailed)
	if assignment.Generation != task.AppGeneration(*appV2) {
		t.Fatalf("assignment generation = %q, want %q", assignment.Generation, task.AppGeneration(*appV2))
	}
}

func TestSubmitStartKeepsFailedGenerationWhenAutoRollbackDisabled(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	appV1 := deployApp("rollout-no-auto", dockerWorkload("web", "nginx:1",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	appV2 := deployApp("rollout-no-auto", dockerWorkload("web", "nginx:2",
		workloadKind(deployv1.WorkloadKindService),
		workloadRollingRollout(),
	))
	harness.submitDeploy(t, appV1)
	if err := harness.svc.SubmitStart(harness.ctx, appV1.Metadata); err != nil {
		t.Fatal(err)
	}

	harness.runtime.failDeployArtifact("nginx:2", errors.New("replacement failed"))
	harness.submitDeploy(t, appV2)
	if err := harness.svc.SubmitStart(harness.ctx, appV2.Metadata); err == nil {
		t.Fatal("expected rollout replacement failure")
	}

	got, ok := harness.raft.GetDesiredDeployApp(appV2.Metadata)
	if !ok {
		t.Fatal("desired app missing after failed rollout")
	}
	if got.Workloads[0].Run.Artifact.Image != "nginx:2" {
		t.Fatalf("desired image after failed rollout = %q", got.Workloads[0].Run.Artifact.Image)
	}
}

func TestSubmitStartHonorsRolloutProgressDeadline(t *testing.T) {
	t.Parallel()

	server := alwaysFailingReadinessServer(t)
	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("rollout-deadline", dockerWorkload("web", "nginx",
		workloadKind(deployv1.WorkloadKindService),
		workloadEndpoint("http", httpServerPort(t, server.URL), deployv1.ProtoHTTP),
		workloadHTTPReadiness("http", "/ready", func(probe *deployv1.Probe) {
			probe.Interval = (200 * time.Millisecond).String()
			probe.Timeout = (100 * time.Millisecond).String()
			probe.Retries = 100
		}),
		workloadProgressDeadline(50*time.Millisecond),
	))
	harness.submitDeploy(t, app)

	started := time.Now()
	if err := harness.svc.SubmitStart(harness.ctx, app.Metadata); err == nil {
		t.Fatal("expected rollout progress deadline failure")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("SubmitStart elapsed = %s, want under 1s", elapsed)
	}
	assignment := harness.requireAssignment(t, app, "web", "node-a", workloadmeta.AssignmentStatusFailed)
	requireAssignmentError(t, assignment)
}

func alwaysFailingReadinessServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	return server
}

func requireRuntimeEvents(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime events = %#v, want %#v", got, want)
	}
}
