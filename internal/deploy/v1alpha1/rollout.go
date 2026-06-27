package v1alpha1

import "strings"

func (r *Rollout) EffectiveStrategy() string {
	if r == nil || strings.TrimSpace(r.Strategy) == "" {
		return RolloutStrategyStopBeforeStart
	}
	return strings.TrimSpace(r.Strategy)
}

func IsRolloutStrategy(strategy string) bool {
	switch strings.TrimSpace(strategy) {
	case "", RolloutStrategyRecreate, RolloutStrategyStopBeforeStart, RolloutStrategyRolling:
		return true
	default:
		return false
	}
}
