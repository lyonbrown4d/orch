package orch_test

import (
	"testing"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestOrchRollout(t *testing.T) {
	app := loadAppString(t, "rollout.orch", `app {
  name = "rollout"
  namespace = "demo"

  service api {
    image = "traefik/whoami:v1.11"
    http(80)

    rollout {
      strategy = "rolling"
      max_unavailable = 1
      max_surge = 2
      rollback_on_failure = true
      progress_deadline = "30s"
    }
  }
}`)

	workload := workloadByName(t, app, "api")
	if workload.Rollout == nil {
		t.Fatal("api rollout is nil")
	}
	if workload.Rollout.Strategy != v1.RolloutStrategyRolling || workload.Rollout.MaxUnavailable != 1 || workload.Rollout.MaxSurge != 2 || !workload.Rollout.RollbackOnFailure || workload.Rollout.ProgressDeadline != "30s" {
		t.Fatalf("api rollout = %+v", workload.Rollout)
	}
	requireValidApp(t, app)
}
