package cmd

import (
	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"

	"github.com/lyonbrown4d/orch/internal/api"
)

func NewDeploySnapshotForTest(total int) *deploySnapshot {
	return newDeploySnapshot(total)
}

func CollectAssignmentSnapshotForTest(snapshot *deploySnapshot, items *list.List[api.AssignmentItem], expectedKeys *set.Set[string], expectedGeneration string) {
	collectAssignmentSnapshot(snapshot, items, expectedKeys, expectedGeneration)
}
