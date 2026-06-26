package task

import (
	"context"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) applyWorkloadAssignment(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID, status, generation, errMsg string, addresses ...string) {
	if s == nil || s.raft == nil {
		return
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = workloadmeta.AssignmentStatusAssigned
	}
	address := ""
	if len(addresses) > 0 {
		address = strings.TrimSpace(addresses[0])
	}
	assignment := workloadmeta.Assignment{
		Key:        workloadmeta.AssignmentKey(meta, workload.Name),
		Metadata:   meta,
		Workload:   workload.Name,
		Node:       strings.TrimSpace(nodeID),
		Address:    address,
		Runtime:    workload.Runtime,
		Artifact:   runconfig.ArtifactSummary(workload.Run),
		Status:     status,
		Generation: strings.TrimSpace(generation),
		Error:      strings.TrimSpace(errMsg),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := s.raft.ApplyWorkloadAssignment(ctx, assignment); err != nil {
		s.logger.Warn("workload assignment apply",
			"error", err,
			"app", meta.Name,
			"workload", workload.Name,
			"node", nodeID,
			"status", status,
		)
	}
}

func (s *Service) localWorkloadAddress(meta deployv1.Metadata, workloadName string) string {
	if s == nil || s.dns == nil {
		return ""
	}
	address, ok := s.dns.LookupLocalWorkloadIPv4(meta.Namespace, workloadName)
	if !ok {
		return ""
	}
	return address
}

func (s *Service) dispatchWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) (DispatchResult, error) {
	if s.dispatcher == nil {
		return DispatchResult{}, oopsx.B("task").Errorf("placement selected node %q for workload %q but worker dispatcher is unavailable", nodeID, workload.Name)
	}
	result, err := s.dispatcher.DispatchWorkload(ctx, nodeID, meta, workload)
	if err != nil {
		return DispatchResult{}, oopsx.B("task").Wrapf(err, "dispatch workload %s to node %s", workload.Name, nodeID)
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "dispatched"
	}
	result.Status = status
	s.logger.Info("workload dispatched", "workload", workload.Name, "node", nodeID, "runtime", workload.Runtime, "status", status)
	return result, nil
}

func (s *Service) deployLocalWorkload(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, nodeID string) error {
	if err := s.runtime.Deploy(ctx, meta, workload); err != nil {
		return oopsx.B("task").Wrapf(err, "deploy workload %s", workload.Name)
	}
	s.registry.Upsert(registry.WorkloadRecord{
		Name:     workload.Name,
		Node:     nodeID,
		Runtime:  string(workload.Runtime),
		Artifact: runconfig.ArtifactSummary(workload.Run),
		Status:   "running",
	})
	return nil
}
