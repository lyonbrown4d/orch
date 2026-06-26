package task

import (
	"context"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func (s *Service) allowWorkloadRestart(workload deployv1.Workload, assignmentKey, generation string) bool {
	if restartPolicyForWorkload(workload) == restartPolicyNever {
		return false
	}
	maxRestarts := workloadMaxRestarts(workload)
	key := restartAttemptKey(assignmentKey, generation)
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	if s.restartAttempts == nil {
		s.restartAttempts = map[string]int{}
	}
	attempts := s.restartAttempts[key]
	if maxRestarts > 0 && attempts >= maxRestarts {
		return false
	}
	s.restartAttempts[key] = attempts + 1
	return true
}

func workloadMaxRestarts(workload deployv1.Workload) int {
	if workload.Lifecycle == nil || workload.Lifecycle.MaxRestarts <= 0 {
		return 0
	}
	return workload.Lifecycle.MaxRestarts
}

func restartAttemptKey(assignmentKey, generation string) string {
	return strings.TrimSpace(assignmentKey) + "@" + strings.TrimSpace(generation)
}

func (s *Service) waitWorkloadRestartDelay(ctx context.Context, workload deployv1.Workload) bool {
	if workload.Lifecycle == nil || strings.TrimSpace(workload.Lifecycle.RestartDelay) == "" {
		return true
	}
	delay, err := time.ParseDuration(workload.Lifecycle.RestartDelay)
	if err != nil || delay <= 0 {
		return true
	}
	if waitErr := sleepContext(ctx, delay); waitErr != nil {
		return false
	}
	return true
}
