package task_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestHealthControllerRunsStartupBeforeLiveness(t *testing.T) {
	t.Parallel()

	var startupReady atomic.Bool
	var livenessHits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/startup":
			if !startupReady.Load() {
				http.Error(w, "starting", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/live":
			livenessHits.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	harness := newTaskHarness(t, config.Default(), nil)
	harness.startReconcile()
	app := deployApp("startup-liveness",
		dockerWorkload("web", "nginx",
			workloadEndpoint("http", httpServerPort(t, server.URL), deployv1.ProtoHTTP),
			workloadHTTPStartup("http", "/startup", fastReadinessProbe()),
			workloadHTTPLiveness("http", "/live", fastReadinessProbe()),
		),
	)

	harness.submitDeploy(t, app)
	requireWorkloadName(t, harness.waitRuntimeDeploy(t, deployReconcileTimeout), "web")
	harness.requireAssignment(t, app, "web", "node-a", workloadmeta.AssignmentStatusRunning)
	requireAtomicValueWithin(t, &livenessHits, 0, 150*time.Millisecond)

	startupReady.Store(true)
	waitForAtomicAtLeast(t, &livenessHits, 1, deployReconcileTimeout)
	if err := harness.svc.SubmitStop(harness.ctx, app.Metadata); err != nil {
		t.Fatal(err)
	}
}

func TestHealthControllerMarksFailedWhenLivenessFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "dead", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	harness := newTaskHarness(t, config.Default(), nil)
	harness.startReconcile()
	app := deployApp("liveness-fail",
		dockerWorkload("web", "nginx",
			workloadEndpoint("http", httpServerPort(t, server.URL), deployv1.ProtoHTTP),
			workloadHTTPLiveness("http", "/live", failingReadinessProbe()),
		),
	)

	harness.submitDeploy(t, app)
	requireWorkloadName(t, harness.waitRuntimeDeploy(t, deployReconcileTimeout), "web")
	assignment := harness.requireAssignment(t, app, "web", "node-a", workloadmeta.AssignmentStatusFailed)
	requireAssignmentError(t, assignment)
	requireRuntimeStop(t, harness, "web", deployReconcileTimeout)
}

func workloadHTTPStartup(endpoint, path string, opts ...func(*deployv1.Probe)) workloadOption {
	return func(workload *deployv1.Workload) {
		probe := &deployv1.Probe{HTTP: &deployv1.HTTPProbe{Path: path, Endpoint: deployv1.EndpointRef{Workload: workload.Name, Endpoint: endpoint}}}
		for _, opt := range opts {
			opt(probe)
		}
		ensureWorkloadHealth(workload).Startup = probe
	}
}

func workloadHTTPLiveness(endpoint, path string, opts ...func(*deployv1.Probe)) workloadOption {
	return func(workload *deployv1.Workload) {
		probe := &deployv1.Probe{HTTP: &deployv1.HTTPProbe{Path: path, Endpoint: deployv1.EndpointRef{Workload: workload.Name, Endpoint: endpoint}}}
		for _, opt := range opts {
			opt(probe)
		}
		ensureWorkloadHealth(workload).Liveness = probe
	}
}

func ensureWorkloadHealth(workload *deployv1.Workload) *deployv1.Health {
	if workload.Health == nil {
		workload.Health = &deployv1.Health{}
	}
	return workload.Health
}

func requireAtomicValueWithin(t *testing.T, value *atomic.Int64, want int64, wait time.Duration) {
	t.Helper()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	<-timer.C
	if got := value.Load(); got != want {
		t.Fatalf("atomic value = %d, want %d", got, want)
	}
}

func waitForAtomicAtLeast(t *testing.T, value *atomic.Int64, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if value.Load() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("atomic value = %d, want at least %d", value.Load(), want)
}

func requireRuntimeStop(t *testing.T, harness *taskHarness, workloadName string, timeout time.Duration) {
	t.Helper()
	select {
	case got := <-harness.runtime.stopCh:
		if got != workloadName {
			t.Fatalf("stopped workload = %q, want %q", got, workloadName)
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for workload %q to stop", workloadName)
	}
}
