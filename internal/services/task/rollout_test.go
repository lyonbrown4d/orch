package task_test

import (
	"errors"
	"reflect"
	"testing"

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

func requireRuntimeEvents(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime events = %#v, want %#v", got, want)
	}
}
