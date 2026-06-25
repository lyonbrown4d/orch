package raftsvc

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// ApplyWorkloadAssignment records the latest scheduler decision/result for one workload.
// Callers must target the Raft leader.
func (s *Service) ApplyWorkloadAssignment(ctx context.Context, assignment workloadmeta.Assignment) error {
	if s == nil {
		return oopsx.B("raft").Errorf("nil service")
	}
	assignment.Key = strings.TrimSpace(assignment.Key)
	assignment.Metadata.Name = strings.TrimSpace(assignment.Metadata.Name)
	assignment.Metadata.Namespace = strings.TrimSpace(assignment.Metadata.Namespace)
	assignment.Workload = strings.TrimSpace(assignment.Workload)
	assignment.Node = strings.TrimSpace(assignment.Node)
	assignment.Address = strings.TrimSpace(assignment.Address)
	assignment.Status = strings.TrimSpace(assignment.Status)
	if assignment.Metadata.Name == "" {
		return oopsx.B("raft").Errorf("assignment metadata.name is required")
	}
	if assignment.Workload == "" {
		return oopsx.B("raft").Errorf("assignment workload is required")
	}
	if assignment.Key == "" {
		assignment.Key = workloadmeta.AssignmentKey(assignment.Metadata, assignment.Workload)
	}
	if assignment.Key == "" {
		return oopsx.B("raft").Errorf("assignment key is required")
	}
	if assignment.UpdatedAt.IsZero() {
		assignment.UpdatedAt = time.Now().UTC()
	}

	b, err := json.Marshal(struct {
		Type       string                  `json:"type"`
		Assignment workloadmeta.Assignment `json:"assignment"`
	}{
		Type:       cmdUpsertWorkloadAssignment,
		Assignment: assignment,
	})
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "marshal workload assignment command")
	}

	return s.applyCommand(ctx, b, 5*time.Second, "not leader: send workload assignment to the raft leader node")
}

// ListWorkloadAssignments returns a stable snapshot of scheduler assignment records.
func (s *Service) ListWorkloadAssignments() *list.List[workloadmeta.Assignment] {
	if s == nil || s.fsm == nil {
		return list.NewList[workloadmeta.Assignment]()
	}
	out := s.fsm.listAssignments()
	out.Sort(func(a, b workloadmeta.Assignment) int {
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

// GetWorkloadAssignment returns the latest scheduler assignment record for key.
func (s *Service) GetWorkloadAssignment(key string) (workloadmeta.Assignment, bool) {
	if s == nil || s.fsm == nil {
		return workloadmeta.Assignment{}, false
	}
	return s.fsm.getAssignment(key)
}
