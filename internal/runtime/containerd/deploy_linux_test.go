//go:build linux

package containerd

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
)

type fakeDNS struct{}

func (fakeDNS) WorkloadNameserver() (string, bool) {
	return "10.0.0.53", true
}

func (fakeDNS) WorkloadSearchDomains(namespace string) *list.List[string] {
	return list.NewList(namespace+".svc.orch.local", "svc.orch.local")
}

type fakeCRIRuntime struct {
	runtimeapi.RuntimeServiceClient

	containers []*runtimeapi.Container
	sandboxes  []*runtimeapi.PodSandbox
	statuses   map[string]*runtimeapi.ContainerStatus

	stoppedContainers []string
	removedContainers []string
	stoppedSandboxes  []string
	removedSandboxes  []string
}

func (f *fakeCRIRuntime) ListContainers(context.Context, *runtimeapi.ListContainersRequest, ...grpc.CallOption) (*runtimeapi.ListContainersResponse, error) {
	return &runtimeapi.ListContainersResponse{Containers: f.containers}, nil
}

func (f *fakeCRIRuntime) ContainerStatus(_ context.Context, req *runtimeapi.ContainerStatusRequest, _ ...grpc.CallOption) (*runtimeapi.ContainerStatusResponse, error) {
	return &runtimeapi.ContainerStatusResponse{Status: f.statuses[req.GetContainerId()]}, nil
}

func (f *fakeCRIRuntime) StopContainer(_ context.Context, req *runtimeapi.StopContainerRequest, _ ...grpc.CallOption) (*runtimeapi.StopContainerResponse, error) {
	f.stoppedContainers = append(f.stoppedContainers, req.GetContainerId())
	return &runtimeapi.StopContainerResponse{}, nil
}

func (f *fakeCRIRuntime) RemoveContainer(_ context.Context, req *runtimeapi.RemoveContainerRequest, _ ...grpc.CallOption) (*runtimeapi.RemoveContainerResponse, error) {
	f.removedContainers = append(f.removedContainers, req.GetContainerId())
	return &runtimeapi.RemoveContainerResponse{}, nil
}

func (f *fakeCRIRuntime) ListPodSandbox(context.Context, *runtimeapi.ListPodSandboxRequest, ...grpc.CallOption) (*runtimeapi.ListPodSandboxResponse, error) {
	return &runtimeapi.ListPodSandboxResponse{Items: f.sandboxes}, nil
}

func (f *fakeCRIRuntime) StopPodSandbox(_ context.Context, req *runtimeapi.StopPodSandboxRequest, _ ...grpc.CallOption) (*runtimeapi.StopPodSandboxResponse, error) {
	f.stoppedSandboxes = append(f.stoppedSandboxes, req.GetPodSandboxId())
	return &runtimeapi.StopPodSandboxResponse{}, nil
}

func (f *fakeCRIRuntime) RemovePodSandbox(_ context.Context, req *runtimeapi.RemovePodSandboxRequest, _ ...grpc.CallOption) (*runtimeapi.RemovePodSandboxResponse, error) {
	f.removedSandboxes = append(f.removedSandboxes, req.GetPodSandboxId())
	return &runtimeapi.RemovePodSandboxResponse{}, nil
}

func TestCRIDNSConfig(t *testing.T) {
	t.Parallel()

	got := criDNSConfig(fakeDNS{}, "demo")
	if got == nil {
		t.Fatal("expected DNS config")
	}
	if !slices.Equal(got.Servers, []string{"10.0.0.53"}) {
		t.Fatalf("servers = %#v", got.Servers)
	}
	if !slices.Equal(got.Searches, []string{"demo.svc.orch.local", "svc.orch.local"}) {
		t.Fatalf("searches = %#v", got.Searches)
	}
}

func TestCRIContainerConfig(t *testing.T) {
	t.Parallel()

	cfg := criContainerConfig("busybox:latest", deployv1.Metadata{Name: "demo", Namespace: "default"}, deployv1.Workload{
		Name:    "api",
		Runtime: deployv1.RuntimeContainerd,
		Run: deployv1.RunSpec{
			Exec: deployv1.ExecSpec{
				Command: []string{"sleep"},
				Args:    []string{"60"},
			},
			Env: []deployv1.EnvVar{{Name: "APP_ENV", Value: "test"}},
		},
		Resources: &deployv1.Resources{CPUMillis: 500, MemoryBytes: 128 << 20},
	})

	if cfg.Image.GetImage() != "busybox:latest" {
		t.Fatalf("image = %q", cfg.Image.GetImage())
	}
	if !slices.Equal(cfg.Command, []string{"sleep"}) || !slices.Equal(cfg.Args, []string{"60"}) {
		t.Fatalf("argv = %#v %#v", cfg.Command, cfg.Args)
	}
	if len(cfg.Envs) != 1 || cfg.Envs[0].Key != "APP_ENV" || string(cfg.Envs[0].Value) != "test" {
		t.Fatalf("envs = %#v", cfg.Envs)
	}
	res := cfg.GetLinux().GetResources()
	if res == nil {
		t.Fatal("expected Linux resources")
	}
	if res.GetMemoryLimitInBytes() != 128<<20 {
		t.Fatalf("memory = %d", res.GetMemoryLimitInBytes())
	}
	if res.GetCpuQuota() == 0 || res.GetCpuPeriod() == 0 {
		t.Fatalf("cpu quota/period = %d/%d", res.GetCpuQuota(), res.GetCpuPeriod())
	}
}

func TestCRISandboxConfigWithoutResolver(t *testing.T) {
	t.Parallel()

	provider := &Provider{root: t.TempDir(), dns: nil}
	cfg := criSandboxConfig(provider, deployv1.Metadata{Name: "demo", Namespace: "prod"}, deployv1.Workload{Name: "api"})
	if cfg.GetDnsConfig() != nil {
		t.Fatalf("dns config = %#v, want nil without resolver", cfg.GetDnsConfig())
	}
	if cfg.GetMetadata().GetNamespace() != "prod" {
		t.Fatalf("namespace = %q", cfg.GetMetadata().GetNamespace())
	}
	if cfg.GetLogDirectory() == "" {
		t.Fatal("expected log directory")
	}
}

func TestCRISandboxConfigWithContainerdNamespace(t *testing.T) {
	t.Parallel()

	appCfg := config.Default()
	appCfg.DNS.Workload.Nameserver = "10.0.0.53"
	provider := &Provider{
		root: t.TempDir(),
		dns:  dnssvc.New(appCfg, slog.New(slog.NewTextHandler(io.Discard, nil))),
	}
	cfg := criSandboxConfig(provider, deployv1.Metadata{Name: "demo", Namespace: "prod"}, deployv1.Workload{
		Name:    "api",
		Runtime: deployv1.RuntimeContainerd,
		Run: deployv1.RunSpec{
			Options: deployv1.RunOptions{
				Containerd: &deployv1.ContainerdOptions{Namespace: "custom-ns"},
			},
		},
	})
	if got := cfg.GetMetadata().GetNamespace(); got != "custom-ns" {
		t.Fatalf("namespace = %q, want %q", got, "custom-ns")
	}
	if got := cfg.GetAnnotations()["orch.io/namespace"]; got != "prod" {
		t.Fatalf("orch annotation namespace = %q, want %q", got, "prod")
	}
	if !slices.Equal(cfg.GetDnsConfig().GetSearches(), []string{"prod.svc.orch.local", "svc.orch.local", "orch.local"}) {
		t.Fatalf("dns searches = %#v", cfg.GetDnsConfig().GetSearches())
	}
}

func TestPrepareExistingCRIWorkloadKeepsRunningContainer(t *testing.T) {
	t.Parallel()

	rt := &fakeCRIRuntime{
		containers: []*runtimeapi.Container{{Id: "ctr-running", PodSandboxId: "sand-running"}},
		statuses: map[string]*runtimeapi.ContainerStatus{
			"ctr-running": {Id: "ctr-running", State: runtimeapi.ContainerState_CONTAINER_RUNNING},
		},
	}
	ready, err := (&Provider{}).prepareExistingCRIWorkload(context.Background(), rt, deployv1.Metadata{Name: "demo", Namespace: "prod"}, deployv1.Workload{Name: "api"})
	if err != nil {
		t.Fatalf("prepare existing running workload: %v", err)
	}
	if !ready {
		t.Fatal("expected existing running workload to be ready")
	}
	if len(rt.removedContainers) > 0 || len(rt.removedSandboxes) > 0 {
		t.Fatalf("unexpected cleanup containers=%v sandboxes=%v", rt.removedContainers, rt.removedSandboxes)
	}
}

func TestPrepareExistingCRIWorkloadRemovesStaleContainerAndSandbox(t *testing.T) {
	t.Parallel()

	rt := &fakeCRIRuntime{
		containers: []*runtimeapi.Container{{Id: "ctr-exited", PodSandboxId: "sand-exited"}},
		sandboxes:  []*runtimeapi.PodSandbox{{Id: "sand-exited"}},
		statuses: map[string]*runtimeapi.ContainerStatus{
			"ctr-exited": {Id: "ctr-exited", State: runtimeapi.ContainerState_CONTAINER_EXITED},
		},
	}
	ready, err := (&Provider{}).prepareExistingCRIWorkload(context.Background(), rt, deployv1.Metadata{Name: "demo", Namespace: "prod"}, deployv1.Workload{Name: "api"})
	if err != nil {
		t.Fatalf("prepare existing stale workload: %v", err)
	}
	if ready {
		t.Fatal("expected stale workload to be cleaned up for redeploy")
	}
	if !slices.Equal(rt.removedContainers, []string{"ctr-exited"}) {
		t.Fatalf("removed containers = %v", rt.removedContainers)
	}
	if !slices.Equal(rt.removedSandboxes, []string{"sand-exited"}) {
		t.Fatalf("removed sandboxes = %v", rt.removedSandboxes)
	}
}

func TestCRIWorkloadLabelSelector(t *testing.T) {
	t.Parallel()

	got := criWorkloadLabelSelector(deployv1.Metadata{Namespace: "prod"}, " api ")
	if got["orch.io/namespace"] != "prod" {
		t.Fatalf("namespace label = %q", got["orch.io/namespace"])
	}
	if got["orch.io/workload"] != "api" {
		t.Fatalf("workload label = %q", got["orch.io/workload"])
	}
}

func TestCRIContainerStateStatus(t *testing.T) {
	t.Parallel()

	tests := map[runtimeapi.ContainerState]string{
		runtimeapi.ContainerState_CONTAINER_CREATED: "created",
		runtimeapi.ContainerState_CONTAINER_RUNNING: "running",
		runtimeapi.ContainerState_CONTAINER_EXITED:  "exited",
		runtimeapi.ContainerState_CONTAINER_UNKNOWN: "unknown",
	}
	for state, want := range tests {
		if got := criContainerStateStatus(state); got != want {
			t.Fatalf("state %s = %q, want %q", state, got, want)
		}
	}
}

func TestCRIContainerLogPath(t *testing.T) {
	t.Parallel()

	meta := deployv1.Metadata{Name: "demo", Namespace: "prod"}
	provider := &Provider{root: t.TempDir()}

	got := criContainerLogPath(provider, meta, "api", &runtimeapi.ContainerStatus{LogPath: "custom.log"})
	want := filepath.Join(provider.logDir(meta, "api"), "custom.log")
	if got != want {
		t.Fatalf("relative log path = %q, want %q", got, want)
	}

	got = criContainerLogPath(provider, meta, "api", &runtimeapi.ContainerStatus{LogPath: "/var/log/orch/api.log"})
	if got != "/var/log/orch/api.log" {
		t.Fatalf("absolute log path = %q", got)
	}

	got = criContainerLogPath(provider, meta, "api", &runtimeapi.ContainerStatus{})
	want = filepath.Join(provider.logDir(meta, "api"), "api.log")
	if got != want {
		t.Fatalf("fallback log path = %q, want %q", got, want)
	}
}
