package v1alpha1

import (
	"fmt"
	"strings"
	"time"
)

func (r *Rollout) EffectiveStrategy() string {
	if r == nil || strings.TrimSpace(r.Strategy) == "" {
		return RolloutStrategyStopBeforeStart
	}
	return strings.TrimSpace(r.Strategy)
}

func (r *Rollout) RollbackOnFailureEnabled() bool {
	return r != nil && r.RollbackOnFailure
}

func (r *Rollout) ProgressDeadlineDuration() (time.Duration, error) {
	if r == nil || strings.TrimSpace(r.ProgressDeadline) == "" {
		return 0, nil
	}
	deadline, err := time.ParseDuration(r.ProgressDeadline)
	if err != nil {
		return 0, fmt.Errorf("parse rollout progress deadline: %w", err)
	}
	return deadline, nil
}

func IsRolloutStrategy(strategy string) bool {
	switch strings.TrimSpace(strategy) {
	case "", RolloutStrategyRecreate, RolloutStrategyStopBeforeStart, RolloutStrategyRolling:
		return true
	default:
		return false
	}
}
