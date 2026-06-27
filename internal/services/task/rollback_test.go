package task_test

import (
	"strings"
	"testing"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/services/task"
)

func TestSubmitRollbackRestoresPreviousDesiredApp(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	appV1 := deployApp("rollback-demo", dockerWorkload("web", "nginx:1"))
	appV2 := deployApp("rollback-demo", dockerWorkload("web", "nginx:2"))
	harness.submitDeploy(t, appV1)
	harness.submitDeploy(t, appV2)

	if err := harness.svc.SubmitRollback(harness.ctx, appV2.Metadata); err != nil {
		t.Fatal(err)
	}
	got, ok := harness.raft.GetDesiredDeployApp(appV1.Metadata)
	if !ok {
		t.Fatal("desired app missing after rollback")
	}
	if got.Workloads[0].Run.Artifact.Image != "nginx:1" {
		t.Fatalf("desired image after rollback = %q", got.Workloads[0].Run.Artifact.Image)
	}
	previous, ok := harness.raft.GetPreviousDeployApp(appV1.Metadata)
	if !ok || previous.App.Workloads[0].Run.Artifact.Image != "nginx:2" {
		t.Fatalf("previous after rollback = %#v ok=%t", previous, ok)
	}
}

func TestSubmitRollbackRequiresPreviousRevision(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("rollback-empty", dockerWorkload("web", "nginx:1"))
	harness.submitDeploy(t, app)

	err := harness.svc.SubmitRollback(harness.ctx, app.Metadata)
	if err == nil || !strings.Contains(err.Error(), "has no rollback revision") {
		t.Fatalf("SubmitRollback() error = %v, want missing revision", err)
	}
}

func TestTaskAppGenerationDelegatesToDeployModel(t *testing.T) {
	app := deployApp("generation-demo", dockerWorkload("web", "nginx"))
	if got, want := task.AppGeneration(*app), deployv1.AppGeneration(*app); got != want || got == "" {
		t.Fatalf("AppGeneration = %q, want %q", got, want)
	}
}
