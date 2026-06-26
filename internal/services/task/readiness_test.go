package task_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestSubmitDeployWaitsForHTTPReadinessBeforeDependent(t *testing.T) {
	t.Parallel()

	var ready atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			http.NotFound(w, r)
			return
		}
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	harness := newTaskHarness(t, config.Default(), nil)
	harness.startReconcile()
	app := deployApp("readiness-order",
		dockerWorkload("api", "api", workloadDependsOn("db")),
		dockerWorkload("db", "postgres",
			workloadEndpoint("http", httpServerPort(t, server.URL), deployv1.ProtoHTTP),
			workloadHTTPReadiness("http", "/ready", fastReadinessProbe()),
		),
	)

	harness.submitDeploy(t, app)
	first := harness.waitRuntimeDeploy(t, deployReconcileTimeout)
	requireWorkloadName(t, first, "db")
	requireNoRuntimeDeployWithin(t, harness, 150*time.Millisecond)

	ready.Store(true)
	second := harness.waitRuntimeDeploy(t, deployReconcileTimeout)
	requireWorkloadName(t, second, "api")
	harness.requireAssignment(t, app, "db", "node-a", workloadmeta.AssignmentStatusRunning)
	harness.requireAssignment(t, app, "api", "node-a", workloadmeta.AssignmentStatusRunning)
}

func TestSubmitDeployWaitsForTCPReadiness(t *testing.T) {
	t.Parallel()

	_, port := startTCPReadinessServer(t)
	harness := newTaskHarness(t, config.Default(), nil)
	harness.startReconcile()
	app := deployApp("tcp-readiness",
		dockerWorkload("cache", "redis",
			workloadEndpoint("redis", port, deployv1.ProtoTCP),
			workloadTCPReadiness("redis", fastReadinessProbe()),
		),
	)

	harness.submitDeploy(t, app)
	got := harness.waitRuntimeDeploy(t, deployReconcileTimeout)
	requireWorkloadName(t, got, "cache")
	harness.requireAssignment(t, app, "cache", "node-a", workloadmeta.AssignmentStatusRunning)
}

func TestSubmitDeployMarksFailedWhenReadinessDoesNotPass(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	harness := newTaskHarness(t, config.Default(), nil)
	harness.startReconcile()
	app := deployApp("readiness-fail",
		dockerWorkload("api", "api", workloadDependsOn("db")),
		dockerWorkload("db", "postgres",
			workloadEndpoint("http", httpServerPort(t, server.URL), deployv1.ProtoHTTP),
			workloadHTTPReadiness("http", "/ready", failingReadinessProbe()),
		),
	)

	harness.submitDeploy(t, app)
	got := harness.waitRuntimeDeploy(t, deployReconcileTimeout)
	requireWorkloadName(t, got, "db")
	assignment := harness.requireAssignment(t, app, "db", "node-a", workloadmeta.AssignmentStatusFailed)
	requireAssignmentError(t, assignment)
	requireNoWorkloadDeployWithin(t, harness, "api", 200*time.Millisecond)
}

func workloadEndpoint(name string, port int, proto deployv1.EndpointProto) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Endpoints = append(workload.Endpoints, deployv1.Endpoint{Name: name, Port: port, Protocol: proto})
	}
}

func workloadHTTPReadiness(endpoint, path string, opts ...func(*deployv1.Probe)) workloadOption {
	return func(workload *deployv1.Workload) {
		probe := &deployv1.Probe{HTTP: &deployv1.HTTPProbe{Path: path, Endpoint: deployv1.EndpointRef{Workload: workload.Name, Endpoint: endpoint}}}
		for _, opt := range opts {
			opt(probe)
		}
		workload.Health = &deployv1.Health{Readiness: probe}
	}
}

func workloadTCPReadiness(endpoint string, opts ...func(*deployv1.Probe)) workloadOption {
	return func(workload *deployv1.Workload) {
		probe := &deployv1.Probe{TCP: &deployv1.TCPProbe{Endpoint: deployv1.EndpointRef{Workload: workload.Name, Endpoint: endpoint}}}
		for _, opt := range opts {
			opt(probe)
		}
		workload.Health = &deployv1.Health{Readiness: probe}
	}
}

func fastReadinessProbe() func(*deployv1.Probe) {
	return func(probe *deployv1.Probe) {
		probe.Interval = (20 * time.Millisecond).String()
		probe.Timeout = (100 * time.Millisecond).String()
		probe.Retries = 100
	}
}

func failingReadinessProbe() func(*deployv1.Probe) {
	return func(probe *deployv1.Probe) {
		probe.Interval = (10 * time.Millisecond).String()
		probe.Timeout = (50 * time.Millisecond).String()
		probe.Retries = 3
	}
}

func requireNoRuntimeDeployWithin(t *testing.T, harness *taskHarness, wait time.Duration) {
	t.Helper()
	select {
	case got := <-harness.runtime.ch:
		t.Fatalf("unexpected workload deploy %q", got.Name)
	case <-time.After(wait):
	}
}

func requireNoWorkloadDeployWithin(t *testing.T, harness *taskHarness, workloadName string, wait time.Duration) {
	t.Helper()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		select {
		case got := <-harness.runtime.ch:
			if got.Name == workloadName {
				t.Fatalf("unexpected workload deploy %q", got.Name)
			}
		case <-deadline.C:
			return
		}
	}
}

func httpServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func startTCPReadinessServer(t *testing.T) (net.Listener, int) {
	t.Helper()
	listener := listenTCPReadiness(t)
	t.Cleanup(func() { closeTCPReadinessListener(t, listener) })
	go acceptTCPReadinessConnections(listener)
	return listener, tcpListenerPort(t, listener)
}

func listenTCPReadiness(t *testing.T) net.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func closeTCPReadinessListener(t *testing.T, listener net.Listener) {
	t.Helper()
	if err := listener.Close(); err != nil {
		t.Logf("close tcp readiness listener: %v", err)
	}
}

func acceptTCPReadinessConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		if err := conn.Close(); err != nil {
			return
		}
	}
}

func tcpListenerPort(t *testing.T, listener net.Listener) int {
	t.Helper()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr = %T", listener.Addr())
	}
	return addr.Port
}
