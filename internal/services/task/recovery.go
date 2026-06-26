package task

import (
	"context"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const failureRecoveryInterval = 5 * time.Second

type restartPolicy string

const (
	restartPolicyAlways    restartPolicy = "always"
	restartPolicyOnFailure restartPolicy = "on-failure"
	restartPolicyNever     restartPolicy = "never"
)

// RecoverySummary describes one local failure recovery scan.
type RecoverySummary struct {
	Checked   int
	Restarted int
	Failed    int
}

// StartFailureRecovery runs a background loop that restarts failed local workloads assigned to this node.
func (s *Service) StartFailureRecovery(ctx context.Context) {
	if !s.canStartFailureRecovery() {
		return
	}
	loopCtx, cancel, runID, ok := s.beginFailureRecovery(ctx)
	if !ok {
		return
	}
	go s.runFailureRecovery(loopCtx, cancel, runID)
}

func (s *Service) canStartFailureRecovery() bool {
	return s != nil && s.raft != nil && s.runtime != nil && s.registry != nil
}

func (s *Service) beginFailureRecovery(ctx context.Context) (context.Context, context.CancelFunc, uint64, bool) {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if s.recoveryCancel != nil {
		return nil, nil, 0, false
	}
	ctx, cancel := context.WithCancel(ctx)
	s.recoveryRun++
	runID := s.recoveryRun
	s.recoveryCancel = cancel
	s.recoveryWG.Add(1)
	return ctx, cancel, runID, true
}

func (s *Service) runFailureRecovery(ctx context.Context, cancel context.CancelFunc, runID uint64) {
	defer cancel()
	defer s.finishFailureRecovery(runID)
	s.logger.Info("failure recovery started")
	s.RecoverLocalFailures(ctx)
	ticker := time.NewTicker(failureRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RecoverLocalFailures(ctx)
		}
	}
}

func (s *Service) finishFailureRecovery(runID uint64) {
	defer s.recoveryWG.Done()
	s.recoveryMu.Lock()
	if s.recoveryRun == runID {
		s.recoveryCancel = nil
	}
	s.recoveryMu.Unlock()
}

// StopFailureRecovery stops the background failure recovery loop.
func (s *Service) StopFailureRecovery(ctx context.Context) error {
	if s == nil {
		return nil
	}
	cancel := s.takeFailureRecoveryCancel()
	if cancel != nil {
		cancel()
	}
	return s.waitFailureRecoveryStopped(ctx)
}

func (s *Service) takeFailureRecoveryCancel() context.CancelFunc {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	cancel := s.recoveryCancel
	s.recoveryCancel = nil
	return cancel
}

func (s *Service) waitFailureRecoveryStopped(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.recoveryWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return oopsx.B("task").Wrapf(ctx.Err(), "stop failure recovery")
	}
}

// RecoverLocalFailures checks local running assignments once and restarts workloads whose runtime status is terminal.
func (s *Service) RecoverLocalFailures(ctx context.Context) RecoverySummary {
	summary := RecoverySummary{}
	if !s.canStartFailureRecovery() {
		return summary
	}
	self := strings.TrimSpace(s.local.String())
	if self == "" {
		return summary
	}
	apps := s.raft.ListDesiredDeployApps()
	apps.Range(func(_ int, app deployv1.App) bool {
		current, keepGoing := s.recoverAppLocalFailures(ctx, app, self)
		summary.add(current)
		return keepGoing
	})
	return summary
}

func (s *Service) recoverAppLocalFailures(ctx context.Context, app deployv1.App, self string) (RecoverySummary, bool) {
	summary := RecoverySummary{}
	generation := AppGeneration(app)
	for i := range app.Workloads {
		current, keepGoing := s.recoverWorkloadFailure(ctx, app, app.Workloads[i], self, generation)
		summary.add(current)
		if !keepGoing {
			return summary, false
		}
	}
	return summary, true
}

func (s *Service) recoverWorkloadFailure(ctx context.Context, app deployv1.App, workload deployv1.Workload, self, generation string) (RecoverySummary, bool) {
	summary := RecoverySummary{}
	assignment, ok := s.localRunningAssignment(app.Metadata, workload, self, generation)
	if !ok || restartPolicyForWorkload(workload) == restartPolicyNever {
		return summary, true
	}
	summary.Checked = 1
	status, err := s.runtime.Status(ctx, workload.Runtime, app.Metadata, workload.Name)
	if err != nil {
		s.logger.Warn("failure recovery status check failed", "error", err, "app", app.Metadata.Name, "workload", workload.Name)
		return summary, true
	}
	if !restartableRuntimeStatus(status.Status) {
		return summary, true
	}
	if !s.allowWorkloadRestart(workload, assignment.Key, generation) {
		s.logger.Warn("failure recovery skipped after restart limit", "app", app.Metadata.Name, "workload", workload.Name)
		return summary, true
	}
	if !s.waitWorkloadRestartDelay(ctx, workload) {
		return summary, false
	}
	if s.recoverLocalWorkload(ctx, app, workload, assignment, generation, status) {
		summary.Restarted = 1
	} else {
		summary.Failed = 1
	}
	return summary, true
}

func (summary *RecoverySummary) add(other RecoverySummary) {
	summary.Checked += other.Checked
	summary.Restarted += other.Restarted
	summary.Failed += other.Failed
}

func (s *Service) localRunningAssignment(meta deployv1.Metadata, workload deployv1.Workload, self, generation string) (workloadmeta.Assignment, bool) {
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workload.Name))
	if !ok {
		return workloadmeta.Assignment{}, false
	}
	if strings.TrimSpace(assignment.Node) != self || strings.TrimSpace(assignment.Status) != workloadmeta.AssignmentStatusRunning {
		return workloadmeta.Assignment{}, false
	}
	assignmentGeneration := strings.TrimSpace(assignment.Generation)
	if assignmentGeneration != "" && assignmentGeneration != strings.TrimSpace(generation) {
		return workloadmeta.Assignment{}, false
	}
	return assignment, true
}

func restartPolicyForWorkload(workload deployv1.Workload) restartPolicy {
	if workload.Lifecycle != nil {
		switch deployv1.RestartPolicy(strings.TrimSpace(string(workload.Lifecycle.RestartPolicy))) {
		case deployv1.RestartPolicyAlways:
			return restartPolicyAlways
		case deployv1.RestartPolicyOnFailure:
			return restartPolicyOnFailure
		case deployv1.RestartPolicyNever:
			return restartPolicyNever
		}
	}
	switch workload.Kind {
	case deployv1.WorkloadKindJob, deployv1.WorkloadKindCron:
		return restartPolicyNever
	case deployv1.WorkloadKindService, deployv1.WorkloadKindWorker, deployv1.WorkloadKindStateful:
		return restartPolicyAlways
	default:
		return restartPolicyAlways
	}
}

func restartableRuntimeStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "dead", "exited", "failed", "stopped":
		return true
	default:
		return false
	}
}

func (s *Service) recoverLocalWorkload(
	ctx context.Context,
	app deployv1.App,
	workload deployv1.Workload,
	assignment workloadmeta.Assignment,
	generation string,
	status runtime.Status,
) bool {
	self := strings.TrimSpace(assignment.Node)
	s.logger.Warn("failure recovery restarting workload",
		"app", app.Metadata.Name,
		"workload", workload.Name,
		"runtime", workload.Runtime,
		"runtime_status", status.Status,
		"message", status.Message,
	)
	result, err := s.runLocalWorkload(ctx, app.Metadata, workload, self)
	if err != nil {
		s.applyWorkloadAssignment(ctx, app.Metadata, workload, self, workloadmeta.AssignmentStatusFailed, generation, err.Error(), assignment.Address)
		if s.metrics != nil {
			s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "failed")
		}
		s.logger.Warn("failure recovery restart failed", "error", err, "app", app.Metadata.Name, "workload", workload.Name)
		return false
	}
	address := strings.TrimSpace(result.Address)
	if address == "" {
		address = strings.TrimSpace(assignment.Address)
	}
	statusText := strings.TrimSpace(result.Status)
	if statusText == "" {
		statusText = workloadmeta.AssignmentStatusRunning
	}
	s.applyWorkloadAssignment(ctx, app.Metadata, workload, self, statusText, generation, "", address)
	if s.metrics != nil {
		s.metrics.IncDeployWorkload(ctx, string(workload.Runtime), "restarted")
	}
	return true
}
