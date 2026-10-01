package cmd_test

import (
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"

	orchcmd "github.com/lyonbrown4d/orch/cmd/orch-cli/cmd"
	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestCollectAssignmentSnapshotRequiresExpectedGeneration(t *testing.T) {
	expectedKeys := set.NewSet("default/demo/api")
	items := list.NewList(
		api.AssignmentItem{Key: "default/demo/api", Status: workloadmeta.AssignmentStatusRunning, Generation: "old"},
		api.AssignmentItem{Key: "default/demo/api", Status: workloadmeta.AssignmentStatusFailed, Generation: "old", Error: "old failed"},
	)

	snapshot := orchcmd.NewDeploySnapshotForTest(1)
	orchcmd.CollectAssignmentSnapshotForTest(snapshot, items, expectedKeys, "new")

	if snapshot.RunningAssignments != 0 {
		t.Fatalf("RunningAssignments = %d, want 0", snapshot.RunningAssignments)
	}
	if snapshot.FailedAssignment != nil {
		t.Fatalf("FailedAssignment = %#v, want nil", snapshot.FailedAssignment)
	}
}

func TestCollectAssignmentSnapshotAcceptsExpectedGeneration(t *testing.T) {
	expectedKeys := set.NewSet("default/demo/api")
	items := list.NewList(
		api.AssignmentItem{Key: "default/demo/api", Status: workloadmeta.AssignmentStatusRunning, Generation: "new"},
	)

	snapshot := orchcmd.NewDeploySnapshotForTest(1)
	orchcmd.CollectAssignmentSnapshotForTest(snapshot, items, expectedKeys, "new")

	if snapshot.RunningAssignments != 1 {
		t.Fatalf("RunningAssignments = %d, want 1", snapshot.RunningAssignments)
	}
}
