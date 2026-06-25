package orch_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcgolabs/plano/compiler"

	orchdsl "github.com/lyonbrown4d/orch/internal/deploy/orch"
	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func newTestOrch(t *testing.T) (*compiler.Compiler, *orchdsl.Orch) {
	t.Helper()
	c, err := orchdsl.NewCompiler()
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchdsl.NewOrch(c)
	if err != nil {
		t.Fatal(err)
	}
	return c, orch
}

func loadAppString(t *testing.T, srcName, src string) *v1.App {
	t.Helper()
	_, orch := newTestOrch(t)
	app, err := orch.LoadAppString(context.Background(), srcName, src)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func loadAppFile(t *testing.T, path string) *v1.App {
	t.Helper()
	_, orch := newTestOrch(t)
	app, err := orch.LoadAppFile(context.Background(), filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func requireValidApp(t *testing.T, app *v1.App) {
	t.Helper()
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func requireMetadata(t *testing.T, app *v1.App, name, namespace string) {
	t.Helper()
	if app.Metadata.Name != name || app.Metadata.Namespace != namespace {
		t.Fatalf("metadata = %+v", app.Metadata)
	}
}

func requireWorkloadCount(t *testing.T, app *v1.App, want int) {
	t.Helper()
	if len(app.Workloads) != want {
		t.Fatalf("workloads = %d, want %d", len(app.Workloads), want)
	}
}

func workloadByName(t *testing.T, app *v1.App, name string) v1.Workload {
	t.Helper()
	for i := range app.Workloads {
		workload := &app.Workloads[i]
		if workload.Name == name {
			return *workload
		}
	}
	t.Fatalf("workload %q not found in %+v", name, app.Workloads)
	return v1.Workload{}
}

func requireKindRuntime(t *testing.T, workload v1.Workload, kind v1.WorkloadKind, runtime v1.RuntimeKind) {
	t.Helper()
	if workload.Kind != kind || workload.Runtime != runtime {
		t.Fatalf("%s kind/runtime = %q/%q", workload.Name, workload.Kind, workload.Runtime)
	}
}

func requireStatefulScheduling(t *testing.T, workload v1.Workload) {
	t.Helper()
	if workload.Scheduling == nil || !workload.Scheduling.Stateful {
		t.Fatalf("%s scheduling = %+v", workload.Name, workload.Scheduling)
	}
}

func requireDockerNetwork(t *testing.T, workload v1.Workload, network string) {
	t.Helper()
	if workload.Run.Options.Docker == nil || workload.Run.Options.Docker.NetworkMode != network {
		t.Fatalf("%s docker options = %+v", workload.Name, workload.Run.Options.Docker)
	}
}

func requireEndpoint(t *testing.T, workload v1.Workload, name string, port int, proto v1.EndpointProto) {
	t.Helper()
	if len(workload.Endpoints) != 1 {
		t.Fatalf("%s endpoints = %+v", workload.Name, workload.Endpoints)
	}
	got := workload.Endpoints[0]
	if got.Name != name || got.Port != port || got.Protocol != proto {
		t.Fatalf("%s endpoints = %+v", workload.Name, workload.Endpoints)
	}
}

func requireResources(t *testing.T, workload v1.Workload, cpuMillis, memoryBytes int64) {
	t.Helper()
	if workload.Resources == nil || workload.Resources.CPUMillis != cpuMillis || workload.Resources.MemoryBytes != memoryBytes {
		t.Fatalf("%s resources = %+v", workload.Name, workload.Resources)
	}
}

func requireDependsOn(t *testing.T, workload v1.Workload, names ...string) {
	t.Helper()
	if len(workload.DependsOn) != len(names) {
		t.Fatalf("%s dependsOn = %+v", workload.Name, workload.DependsOn)
	}
	for i, name := range names {
		if workload.DependsOn[i].Name != name {
			t.Fatalf("%s dependsOn = %+v", workload.Name, workload.DependsOn)
		}
	}
}

func requireEnv(t *testing.T, workload v1.Workload, name, value string) {
	t.Helper()
	if got := envValue(workload, name); got != value {
		t.Fatalf("%s %s = %q", workload.Name, name, got)
	}
}

func envValue(workload v1.Workload, name string) string {
	for _, env := range workload.Run.Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

func requireIngressRoute(t *testing.T, app *v1.App, idx int, path, workload, endpoint string) {
	t.Helper()
	if len(app.Ingresses) != 1 || len(app.Ingresses[0].Routes) <= idx {
		t.Fatalf("ingresses = %+v", app.Ingresses)
	}
	route := app.Ingresses[0].Routes[idx]
	if route.Path != path || route.Backend.Workload != workload || route.Backend.Endpoint != endpoint {
		t.Fatalf("route = %+v", route)
	}
}

func requireMount(t *testing.T, workload v1.Workload, volume, target string) {
	t.Helper()
	for _, mount := range workload.Mounts {
		if mount.Volume.Name == volume && mount.Target == target {
			return
		}
	}
	t.Fatalf("%s mounts = %+v, want %s -> %s", workload.Name, workload.Mounts, volume, target)
}
