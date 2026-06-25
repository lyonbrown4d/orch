package orch_test

import (
	"testing"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestOrchLoadFullstackDockerExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/fullstack-docker.orch")
	requireMetadata(t, app, "fullstack", "demo")
	requireWorkloadCount(t, app, 4)
	if len(app.Volumes) != 2 {
		t.Fatalf("volumes = %+v", app.Volumes)
	}
	requireFullstackPostgres(t, workloadByName(t, app, "postgres"))
	requireFullstackBackend(t, workloadByName(t, app, "backend"))
	requireIngressRoute(t, app, 0, "/api", "backend", "http")
	requireIngressRoute(t, app, 1, "/", "frontend", "http")
	requireValidApp(t, app)
}

func requireFullstackPostgres(t *testing.T, postgres v1.Workload) {
	t.Helper()
	requireKindRuntime(t, postgres, v1.WorkloadKindStateful, v1.RuntimeDocker)
	requireStatefulScheduling(t, postgres)
	requireDockerNetwork(t, postgres, "orch-demo")
	requireEndpoint(t, postgres, "tcp-5432", 5432, v1.ProtoTCP)
	requireResources(t, postgres, 500, 536870912)
	requireMount(t, postgres, "postgresData", "/var/lib/postgresql/data")
}

func requireFullstackBackend(t *testing.T, backend v1.Workload) {
	t.Helper()
	requireKindRuntime(t, backend, v1.WorkloadKindService, v1.RuntimeDocker)
	requireDockerNetwork(t, backend, "orch-demo")
	requireDependsOn(t, backend, "postgres", "redis")
	requireEndpoint(t, backend, "http", 8080, v1.ProtoHTTP)
	requireResources(t, backend, 500, 536870912)
	requireEnv(t, backend, "HTTP_ADDR", ":8080")
}
