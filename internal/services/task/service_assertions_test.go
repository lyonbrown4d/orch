package task_test

import (
	"testing"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func requireAssignmentPayload(t *testing.T, assignment workloadmeta.Assignment, runtime deployv1.RuntimeKind, artifact string) {
	t.Helper()
	if assignment.Runtime != runtime || assignment.Artifact != artifact {
		t.Fatalf("assignment payload = %#v", assignment)
	}
}

func waitDispatchSignal(t *testing.T, dispatchCh <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-dispatchCh:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for worker dispatch")
	}
}
