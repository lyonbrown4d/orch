package task

import (
	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

// ListWorkloadAssignments returns scheduler assignment state replicated through Raft.
func (s *Service) ListWorkloadAssignments() *list.List[workloadmeta.Assignment] {
	if s == nil || s.raft == nil {
		return list.NewList[workloadmeta.Assignment]()
	}
	return s.raft.ListWorkloadAssignments()
}

// ListVolumeBindings returns replicated local runtime volume attachment state.
func (s *Service) ListVolumeBindings() *list.List[volumemeta.Binding] {
	if s == nil || s.raft == nil {
		return list.NewList[volumemeta.Binding]()
	}
	return s.raft.ListVolumeBindings()
}
