package cmd

import (
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"

	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestCollectAssignmentSnapshotRequiresExpectedGeneration(t *testing.T) {
	expectedKeys := set.NewSet("default/demo/api")
	items := list.NewList(
		api.AssignmentItem{Key: "default/demo/api", Status: workloadmeta.AssignmentStatusRunning, Generation: "old"},
		api.AssignmentItem{Key: "default/demo/api", Status: workloadmeta.AssignmentStatusFailed, Generation: "old", Error: "old failed"},
	)

	snapshot := newDeploySnapshot(1)
	collectAssignmentSnapshot(snapshot, items, expectedKeys, "new")

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

	snapshot := newDeploySnapshot(1)
	collectAssignmentSnapshot(snapshot, items, expectedKeys, "new")

	if snapshot.RunningAssignments != 1 {
		t.Fatalf("RunningAssignments = %d, want 1", snapshot.RunningAssignments)
	}
}
