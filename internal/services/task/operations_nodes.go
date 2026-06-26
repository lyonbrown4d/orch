package task

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) currentWorkloadNode(meta deployv1.Metadata, workloadName string) string {
	return assignmentNodeOrEmpty(s.raft, meta, workloadName)
}

func (s *Service) currentWorkloadAssignmentNode(meta deployv1.Metadata, workloadName string) (string, bool) {
	if s == nil || s.raft == nil {
		return "", false
	}
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workloadName))
	if !ok {
		return "", false
	}
	return strings.TrimSpace(assignment.Node), true
}

func (s *Service) workloadAssignmentStatus(meta deployv1.Metadata, workloadName string) string {
	if s == nil || s.raft == nil {
		return ""
	}
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workloadName))
	if !ok {
		return ""
	}
	return strings.TrimSpace(assignment.Status)
}

func (s *Service) chooseFailoverNode(ctx context.Context, workload deployv1.Workload, current string) (string, error) {
	if err := s.catalog.RefreshLocal(ctx, s.local, s.cfg); err != nil {
		s.logger.Warn("refresh local node capacity before failover", "error", err)
	}
	candidates := s.alternativeNodeCandidates(current)
	if candidates.Len() == 0 {
		return "", oopsx.B("task").Errorf("no alternative node available for workload %q", workload.Name)
	}
	candidate := failoverCandidate(workload, candidates)
	return s.chooseWorkloadNode(ctx, candidate, s.local.String())
}

func failoverCandidate(workload deployv1.Workload, candidates *list.List[string]) deployv1.Workload {
	candidate := workload
	scheduling := deployv1.Scheduling{}
	if workload.Scheduling != nil {
		scheduling = *workload.Scheduling
	}
	scheduling.PreferredNodes = candidates.Values()
	candidate.Scheduling = &scheduling
	return candidate
}

func (s *Service) alternativeNodeCandidates(current string) *list.List[string] {
	current = strings.TrimSpace(current)
	seen := set.NewSet[string]()
	out := list.NewList[string]()
	addNodeCandidate(out, seen, current, s.catalog.NodeIDs().Values()...)
	addNodeCandidate(out, seen, current, sortedConfiguredNodeIDs(s.cfg.Cluster.Nodes)...)
	addNodeCandidate(out, seen, current, s.local.String())
	return out
}

func addNodeCandidate(out *list.List[string], seen *set.Set[string], current string, nodeIDs ...string) {
	for _, raw := range nodeIDs {
		nodeID := strings.TrimSpace(raw)
		if nodeID == "" || nodeID == current {
			continue
		}
		if seen.Contains(nodeID) {
			continue
		}
		seen.Add(nodeID)
		out.Add(nodeID)
	}
}

func sortedConfiguredNodeIDs(nodes map[string]string) []string {
	nodeIDs := list.NewList(mapping.NewMapFrom(nodes).Keys()...)
	nodeIDs.Sort(strings.Compare)
	return nodeIDs.Values()
}

type workloadRunResult struct {
	Status  string
	Address string
}

func (s *Service) runWorkloadOnNode(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) (workloadRunResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return workloadRunResult{}, oopsx.B("task").Errorf("empty target node for workload %q", workload.Name)
	}
	if nodeID == s.local.String() {
		return s.runLocalWorkload(ctx, meta, workload, nodeID)
	}
	return s.runRemoteWorkload(ctx, meta, workload, nodeID)
}

func (s *Service) runRemoteWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) (workloadRunResult, error) {
	result, err := s.dispatchWorkload(ctx, meta, workload, nodeID)
	if err != nil {
		return workloadRunResult{}, err
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "dispatched"
	}
	s.registry.Upsert(registry.WorkloadRecord{
		Name:     workload.Name,
		Node:     nodeID,
		Runtime:  string(workload.Runtime),
		Artifact: runconfig.ArtifactSummary(workload.Run),
		Status:   status,
	})
	return workloadRunResult{Status: status, Address: strings.TrimSpace(result.Address)}, nil
}

func (s *Service) runLocalWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) (workloadRunResult, error) {
	if err := s.deployLocalWorkload(ctx, meta, workload, nodeID); err != nil {
		return workloadRunResult{}, err
	}
	address := s.localWorkloadAddress(meta, workload.Name)
	if err := s.waitWorkloadReadiness(ctx, meta, workload, address); err != nil {
		if stopErr := s.stopLocalWorkload(ctx, meta, workload); stopErr != nil {
			s.logger.Warn("stop workload after readiness failure", "workload", workload.Name, "error", stopErr)
		}
		return workloadRunResult{}, err
	}
	return workloadRunResult{Status: workloadmeta.AssignmentStatusRunning, Address: address}, nil
}
