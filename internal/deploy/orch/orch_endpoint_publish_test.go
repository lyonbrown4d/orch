package orch_test

import (
	"testing"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestOrchEndpointHostPublish(t *testing.T) {
	app := loadAppString(t, "endpoint-publish.orch", `app {
  name = "endpoint-publish"
  namespace = "demo"

  service api {
    image = "nginx:alpine"
    http(80, "http", 8080, "127.0.0.1")
  }

  service db {
    image = "postgres:16-alpine"
    endpoint postgres {
      port = 5432
      protocol = "tcp"
      host_port = 5432
      host_ip = "0.0.0.0"
    }
  }
}`)

	api := workloadByName(t, app, "api")
	requireEndpointPublish(t, api, "http", 80, v1.ProtoHTTP, 8080, "127.0.0.1")
	db := workloadByName(t, app, "db")
	requireEndpointPublish(t, db, "postgres", 5432, v1.ProtoTCP, 5432, "0.0.0.0")
	requireValidApp(t, app)
}

func requireEndpointPublish(t *testing.T, workload v1.Workload, name string, port int, proto v1.EndpointProto, hostPort int, hostIP string) {
	t.Helper()
	if len(workload.Endpoints) != 1 {
		t.Fatalf("%s endpoints = %+v", workload.Name, workload.Endpoints)
	}
	got := workload.Endpoints[0]
	if got.Name != name || got.Port != port || got.Protocol != proto || got.HostPort != hostPort || got.HostIP != hostIP {
		t.Fatalf("%s endpoints = %+v", workload.Name, workload.Endpoints)
	}
}
