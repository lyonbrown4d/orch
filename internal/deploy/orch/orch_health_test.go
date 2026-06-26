package orch_test

import (
	"testing"
	"time"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestOrchHealthReadinessBlocks(t *testing.T) {
	app := loadAppString(t, "health.orch", `app {
  name = "health"
  namespace = "demo"

  stateful db {
    image = "postgres:16-alpine"
    tcp(5432)

    health {
      readiness {
        tcp = true
        endpoint = "tcp-5432"
        interval = "1s"
        timeout = "500ms"
        retries = 60
        start_period = "2s"
      }
    }
  }

  service api {
    image = "ghcr.io/acme/api:latest"
    depends_on = [db]
    http(8080, "http")

    health {
      readiness {
        http = "/ready"
        endpoint = "http"
        interval = "500ms"
        timeout = "250ms"
        retries = 20
      }
    }
  }
}`)

	db := workloadByName(t, app, "db")
	requireTCPReadiness(t, db, "tcp-5432", 0, time.Second.String(), (500 * time.Millisecond).String(), 60, (2 * time.Second).String())

	api := workloadByName(t, app, "api")
	requireHTTPReadiness(t, api, "http", "/ready", 0, (500 * time.Millisecond).String(), (250 * time.Millisecond).String(), 20)
	requireValidApp(t, app)
}

func TestOrchHealthDirectFieldsDefaultToReadiness(t *testing.T) {
	app := loadAppString(t, "health-short.orch", `app {
  name = "health-short"
  namespace = "demo"

  service web {
    image = "nginx:alpine"
    http(80)

    health {
      http = "/"
      endpoint = "http"
      retries = 5
    }
  }
}`)

	web := workloadByName(t, app, "web")
	requireHTTPReadiness(t, web, "http", "/", 0, "", "", 5)
	requireValidApp(t, app)
}

func requireHTTPReadiness(t *testing.T, workload v1.Workload, endpoint, path string, port int, interval, timeout string, retries int) {
	t.Helper()
	if workload.Health == nil || workload.Health.Readiness == nil || workload.Health.Readiness.HTTP == nil {
		t.Fatalf("%s readiness = %+v", workload.Name, workload.Health)
	}
	probe := workload.Health.Readiness
	if probe.HTTP.Endpoint.Endpoint != endpoint || probe.HTTP.Path != path || probe.HTTP.Port != port {
		t.Fatalf("%s http readiness = %+v", workload.Name, probe.HTTP)
	}
	if probe.Interval != interval || probe.Timeout != timeout || probe.Retries != retries {
		t.Fatalf("%s readiness timing = %+v", workload.Name, probe)
	}
}

func requireTCPReadiness(t *testing.T, workload v1.Workload, endpoint string, port int, interval, timeout string, retries int, startPeriod string) {
	t.Helper()
	if workload.Health == nil || workload.Health.Readiness == nil || workload.Health.Readiness.TCP == nil {
		t.Fatalf("%s readiness = %+v", workload.Name, workload.Health)
	}
	probe := workload.Health.Readiness
	if probe.TCP.Endpoint.Endpoint != endpoint || probe.TCP.Port != port {
		t.Fatalf("%s tcp readiness = %+v", workload.Name, probe.TCP)
	}
	if probe.Interval != interval || probe.Timeout != timeout || probe.Retries != retries || probe.StartPeriod != startPeriod {
		t.Fatalf("%s readiness timing = %+v", workload.Name, probe)
	}
}
