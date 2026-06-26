package orch_test

import (
	"testing"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestOrchLifecycleRestartPolicy(t *testing.T) {
	app := loadAppString(t, "lifecycle.orch", `app {
  name = "lifecycle"
  namespace = "demo"

  service api {
    image = "nginx:alpine"
    restart_policy = "on-failure"
    max_restarts = 3
    restart_delay = "2s"
    http(80)
  }

  stateful db {
    image = "postgres:16-alpine"
    tcp(5432)

    lifecycle {
      restart_policy = "always"
      max_restarts = 10
      restart_delay = "5s"
    }
  }
}`)

	requireLifecycle(t, workloadByName(t, app, "api"), v1.RestartPolicyOnFailure, 3, "2s")
	requireLifecycle(t, workloadByName(t, app, "db"), v1.RestartPolicyAlways, 10, "5s")
	requireValidApp(t, app)
}

func requireLifecycle(t *testing.T, workload v1.Workload, policy v1.RestartPolicy, maxRestarts int, restartDelay string) {
	t.Helper()
	if workload.Lifecycle == nil {
		t.Fatalf("%s lifecycle is nil", workload.Name)
	}
	if workload.Lifecycle.RestartPolicy != policy || workload.Lifecycle.MaxRestarts != maxRestarts || workload.Lifecycle.RestartDelay != restartDelay {
		t.Fatalf("%s lifecycle = %+v", workload.Name, workload.Lifecycle)
	}
}
