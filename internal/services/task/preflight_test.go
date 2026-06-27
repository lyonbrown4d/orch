package task_test

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestSubmitDeployRejectsDuplicateHostPortsInApp(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("port-dupe",
		dockerWorkload("web", "nginx", workloadHostPortEndpoint("http", 80, deployv1.ProtoHTTP, 8080, "")),
		dockerWorkload("api", "nginx", workloadHostPortEndpoint("tcp", 8080, deployv1.ProtoTCP, 8080, "")),
	)

	err := harness.svc.SubmitDeploy(harness.ctx, app)
	if err == nil || !strings.Contains(err.Error(), "both claim 0.0.0.0/tcp:8080") {
		t.Fatalf("SubmitDeploy() error = %v, want duplicate host port", err)
	}
}

func TestSubmitDeployRejectsHostPortClaimedByExistingApp(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	existing := deployApp("first",
		dockerWorkload("web", "nginx", workloadHostPortEndpoint("http", 80, deployv1.ProtoHTTP, 8080, "127.0.0.1")),
	)
	harness.applyApp(t, existing)

	next := deployApp("second",
		dockerWorkload("api", "nginx", workloadHostPortEndpoint("tcp", 8080, deployv1.ProtoTCP, 8080, "127.0.0.1")),
	)
	err := harness.svc.SubmitDeploy(harness.ctx, next)
	if err == nil || !strings.Contains(err.Error(), "already claimed by default/first") {
		t.Fatalf("SubmitDeploy() error = %v, want existing app host port conflict", err)
	}
}

func TestSubmitDeployRejectsUnavailableLocalHostPort(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	hostPort, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}

	harness := newTaskHarness(t, config.Default(), nil)
	app := deployApp("port-busy",
		dockerWorkload("web", "nginx", workloadHostPortEndpoint("http", 80, deployv1.ProtoHTTP, hostPort, "127.0.0.1")),
	)

	err = harness.svc.SubmitDeploy(harness.ctx, app)
	if err == nil || !strings.Contains(err.Error(), "host port unavailable") {
		t.Fatalf("SubmitDeploy() error = %v, want unavailable local host port", err)
	}
}

func TestSubmitDeployAllowsSameAppToKeepHostPort(t *testing.T) {
	t.Parallel()

	harness := newTaskHarness(t, config.Default(), nil)
	existing := deployApp("same-app",
		dockerWorkload("web", "nginx:1", workloadHostPortEndpoint("http", 80, deployv1.ProtoHTTP, 8080, "")),
	)
	harness.applyApp(t, existing)

	updated := deployApp("same-app",
		dockerWorkload("web", "nginx:2", workloadHostPortEndpoint("http", 80, deployv1.ProtoHTTP, 8080, "")),
	)
	if err := harness.svc.SubmitDeploy(harness.ctx, updated); err != nil {
		t.Fatalf("SubmitDeploy() same app update = %v", err)
	}
}

func workloadHostPortEndpoint(name string, port int, proto deployv1.EndpointProto, hostPort int, hostIP string) workloadOption {
	return func(workload *deployv1.Workload) {
		workload.Endpoints = append(workload.Endpoints, deployv1.Endpoint{
			Name:     name,
			Port:     port,
			Protocol: proto,
			HostPort: hostPort,
			HostIP:   hostIP,
		})
	}
}
