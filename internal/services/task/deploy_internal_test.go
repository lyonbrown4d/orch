package task

import (
	"testing"

	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestAssignmentConvergedOnDifferentNode(t *testing.T) {
	t.Parallel()

	assignment := workloadmeta.Assignment{
		Node:       "node-a",
		Status:     workloadmeta.AssignmentStatusRunning,
		Generation: "gen-1",
	}
	if !assignmentConvergedOnDifferentNode(assignment, "node-b", "gen-1") {
		t.Fatal("expected running assignment on a different node to skip stale failure")
	}
	if assignmentConvergedOnDifferentNode(assignment, "node-a", "gen-1") {
		t.Fatal("expected same failed node to remain a real failure")
	}
	if assignmentConvergedOnDifferentNode(assignment, "node-b", "gen-2") {
		t.Fatal("expected generation mismatch to remain a real failure")
	}

	assignment.Status = workloadmeta.AssignmentStatusFailed
	if assignmentConvergedOnDifferentNode(assignment, "node-b", "gen-1") {
		t.Fatal("expected failed assignment to remain a real failure")
	}
}
