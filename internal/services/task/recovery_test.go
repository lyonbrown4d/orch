package task_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestRecoverLocalFailuresRestartsExitedService(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("recover-service", dockerWorkload("web", "nginx", workloadKind(deployv1.WorkloadKindService)))
	harness.applyApp(t, app)
	generation := task.AppGeneration(*app)
	applyLocalAssignment(t, harness, app, "web", workloadmeta.AssignmentStatusRunning, generation)
	harness.runtime.setStatus("web", "exited")

	summary := harness.svc.RecoverLocalFailures(harness.ctx)
	if summary.Checked != 1 || summary.Restarted != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	restarted := harness.waitRuntimeDeploy(t, deployReconcileTimeout)
	requireWorkloadName(t, restarted, "web")
	harness.requireAssignment(t, app, "web", "node-a", workloadmeta.AssignmentStatusRunning)
}

func TestRecoverLocalFailuresDoesNotRestartJob(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("recover-job", dockerWorkload("once", "busybox", workloadKind(deployv1.WorkloadKindJob)))
	harness.applyApp(t, app)
	generation := task.AppGeneration(*app)
	applyLocalAssignment(t, harness, app, "once", workloadmeta.AssignmentStatusRunning, generation)
	harness.runtime.setStatus("once", "exited")

	summary := harness.svc.RecoverLocalFailures(harness.ctx)
	if summary.Checked != 0 || summary.Restarted != 0 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	requireNoRuntimeDeployWithin(t, harness, 100*time.Millisecond)
}

func TestRecoverLocalFailuresMarksFailedWhenRestartFails(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("recover-fail", dockerWorkload("web", "nginx", workloadKind(deployv1.WorkloadKindService)))
	harness.applyApp(t, app)
	generation := task.AppGeneration(*app)
	applyLocalAssignment(t, harness, app, "web", workloadmeta.AssignmentStatusRunning, generation)
	harness.runtime.setStatus("web", "failed")
	harness.runtime.failDeploy(errors.New("boom"))

	summary := harness.svc.RecoverLocalFailures(harness.ctx)
	if summary.Checked != 1 || summary.Restarted != 0 || summary.Failed != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	assignment := harness.requireAssignment(t, app, "web", "node-a", workloadmeta.AssignmentStatusFailed)
	requireAssignmentError(t, assignment)
	requireNoRuntimeDeployWithin(t, harness, 100*time.Millisecond)
}

func applyLocalAssignment(t *testing.T, harness *taskHarness, app *deployv1.App, workloadName, status, generation string) {
	t.Helper()
	if err := harness.raft.ApplyWorkloadAssignment(context.Background(), workloadmeta.Assignment{
		Key:        workloadmeta.AssignmentKey(app.Metadata, workloadName),
		Metadata:   app.Metadata,
		Workload:   workloadName,
		Node:       "node-a",
		Runtime:    deployv1.RuntimeDocker,
		Status:     status,
		Generation: generation,
	}); err != nil {
		t.Fatal(err)
	}
}
