package v1alpha1_test

import (
	"strings"
	"testing"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestRolloutEffectiveStrategyDefaultsToStopBeforeStart(t *testing.T) {
	if got := (*deployv1.Rollout)(nil).EffectiveStrategy(); got != deployv1.RolloutStrategyStopBeforeStart {
		t.Fatalf("nil EffectiveStrategy() = %q, want %q", got, deployv1.RolloutStrategyStopBeforeStart)
	}
	rollout := &deployv1.Rollout{}
	if got := rollout.EffectiveStrategy(); got != deployv1.RolloutStrategyStopBeforeStart {
		t.Fatalf("empty EffectiveStrategy() = %q, want %q", got, deployv1.RolloutStrategyStopBeforeStart)
	}
}

func TestValidateWorkloadRollout(t *testing.T) {
	tests := []struct {
		name    string
		rollout deployv1.Rollout
		want    string
	}{
		{name: "invalid strategy", rollout: deployv1.Rollout{Strategy: "blue-green"}, want: "rollout.strategy is invalid"},
		{name: "negative max unavailable", rollout: deployv1.Rollout{Strategy: deployv1.RolloutStrategyRolling, MaxUnavailable: -1}, want: "rollout.maxUnavailable must be >= 0"},
		{name: "negative max surge", rollout: deployv1.Rollout{Strategy: deployv1.RolloutStrategyRolling, MaxSurge: -1}, want: "rollout.maxSurge must be >= 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := validApp()
			app.Workloads[0].Rollout = &tt.rollout

			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}

	app := validApp()
	app.Workloads[0].Rollout = &deployv1.Rollout{Strategy: deployv1.RolloutStrategyStopBeforeStart}
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with rollout = %v", err)
	}
}
