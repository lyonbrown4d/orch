//go:build linux

package containerd

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const (
	criDialTimeout       = 5 * time.Second
	criSandboxIPAttempts = 40
	criSandboxIPDelay    = 50 * time.Millisecond
	criStopTimeout       = int64(10)
)

var errSandboxIPPending = errors.New("sandbox ip pending")

func containerdSocket() string {
	if v := strings.TrimSpace(os.Getenv("CONTAINERD_ADDRESS")); v != "" {
		return strings.TrimPrefix(strings.TrimSpace(v), "unix://")
	}
	return "/run/containerd/containerd.sock"
}

type criClients struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
	image   runtimeapi.ImageServiceClient
}

func dialCRI(ctx context.Context, socket string) (*criClients, error) {
	socket = strings.TrimPrefix(strings.TrimSpace(socket), "unix://")
	if socket == "" {
		socket = containerdSocket()
	}
	dialCtx, cancel := context.WithTimeout(ctx, criDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		"unix://"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI connect %s", socket)
	}
	return &criClients{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
	}, nil
}

func (c *criClients) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func criSandboxID(meta deployv1.Metadata, workloadName string) string {
	return workloadmeta.OrchContainerName(meta, workloadName)
}

func containerdSandboxNamespace(meta deployv1.Metadata, w deployv1.Workload) string {
	if w.Run.Options.Containerd != nil {
		if ns := strings.TrimSpace(w.Run.Options.Containerd.Namespace); ns != "" {
			return ns
		}
	}
	return workloadmeta.NamespaceOrDefault(meta.Namespace)
}

func (p *Provider) logDir(meta deployv1.Metadata, workloadName string) string {
	return filepath.Join(p.rootOrDefault(), "logs", criSandboxID(meta, workloadName))
}

func (p *Provider) rootOrDefault() string {
	if strings.TrimSpace(p.root) != "" {
		return filepath.Clean(p.root)
	}
	return filepath.Join(config.DefaultDataRoot(), "runtime", "containerd")
}

func criWorkloadLabels(meta deployv1.Metadata, w deployv1.Workload) map[string]string {
	return workloadmeta.Labels(meta, w)
}

func criWorkloadLabelSelector(meta deployv1.Metadata, workloadName string) map[string]string {
	return map[string]string{
		"orch.io/namespace": workloadmeta.NamespaceOrDefault(meta.Namespace),
		"orch.io/workload":  strings.TrimSpace(workloadName),
	}
}

func criDNSConfig(dns workloadDNSResolver, namespace string) *runtimeapi.DNSConfig {
	if dns == nil {
		return nil
	}
	nameserver, ok := dns.WorkloadNameserver()
	if !ok {
		return nil
	}
	cfg := &runtimeapi.DNSConfig{Servers: []string{nameserver}}
	if search := dns.WorkloadSearchDomains(namespace); search.Len() > 0 {
		cfg.Searches = search.Values()
	}
	return cfg
}

type workloadDNSResolver interface {
	WorkloadNameserver() (string, bool)
	WorkloadSearchDomains(namespace string) *list.List[string]
}

func criSandboxConfig(p *Provider, meta deployv1.Metadata, w deployv1.Workload) *runtimeapi.PodSandboxConfig {
	appNS := workloadmeta.NamespaceOrDefault(meta.Namespace)
	sandboxNS := containerdSandboxNamespace(meta, w)
	name := criSandboxID(meta, w.Name)
	return &runtimeapi.PodSandboxConfig{
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      name,
			Uid:       name,
			Namespace: sandboxNS,
			Attempt:   0,
		},
		Hostname:     workloadmeta.SanitizeName(w.Name),
		LogDirectory: p.logDir(meta, w.Name),
		DnsConfig:    criDNSConfig(p.dns, appNS),
		Labels:       criWorkloadLabels(meta, w),
		Annotations: map[string]string{
			"orch.io/app":       meta.Name,
			"orch.io/namespace": appNS,
			"orch.io/workload":  w.Name,
		},
		Linux: &runtimeapi.LinuxPodSandboxConfig{},
	}
}

func criEnv(vars *list.List[deployv1.EnvVar]) []*runtimeapi.KeyValue {
	return list.FilterMapList(vars, func(_ int, v deployv1.EnvVar) (*runtimeapi.KeyValue, bool) {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			return nil, false
		}
		return &runtimeapi.KeyValue{Key: name, Value: []byte(v.Value)}, true
	}).Values()
}

func criLinuxResources(w deployv1.Workload) *runtimeapi.LinuxContainerResources {
	if w.Resources == nil {
		return nil
	}
	res := &runtimeapi.LinuxContainerResources{}
	if w.Resources.MemoryBytes > 0 {
		res.MemoryLimitInBytes = w.Resources.MemoryBytes
	}
	if w.Resources.CPUMillis > 0 {
		quota, period := runconfig.CFSQuota(w.Resources.CPUMillis)
		res.CpuQuota = quota
		res.CpuPeriod = int64(period)
	}
	if res.MemoryLimitInBytes == 0 && res.CpuQuota == 0 && res.CpuPeriod == 0 {
		return nil
	}
	return res
}

func criContainerMounts(p *Provider, meta deployv1.Metadata, w deployv1.Workload) ([]*runtimeapi.Mount, error) {
	mounts, err := runconfig.LocalMounts(criMountRoot(p), meta, w)
	if err != nil {
		return nil, err
	}
	if err := runconfig.EnsureLocalMountSources(mounts); err != nil {
		return nil, err
	}
	return list.MapList(mounts, func(_ int, mount runconfig.Mount) *runtimeapi.Mount {
		return &runtimeapi.Mount{
			ContainerPath: mount.Target,
			HostPath:      mount.SourcePath,
			Readonly:      mount.ReadOnly,
		}
	}).Values(), nil
}

func criMountRoot(p *Provider) string {
	if p == nil {
		return filepath.Join(config.DefaultDataRoot(), "runtime", "containerd")
	}
	return p.rootOrDefault()
}

func criContainerConfig(p *Provider, ref string, meta deployv1.Metadata, w deployv1.Workload) (*runtimeapi.ContainerConfig, error) {
	mounts, err := criContainerMounts(p, meta, w)
	if err != nil {
		return nil, err
	}
	cfg := &runtimeapi.ContainerConfig{
		Metadata: &runtimeapi.ContainerMetadata{
			Name:    workloadmeta.SanitizeName(w.Name),
			Attempt: 0,
		},
		Image:      &runtimeapi.ImageSpec{Image: ref},
		Command:    w.Run.Exec.Command,
		Args:       w.Run.Exec.Args,
		WorkingDir: strings.TrimSpace(w.Run.Cwd),
		Envs:       criEnv(w.EnvList()),
		Labels:     criWorkloadLabels(meta, w),
		LogPath:    workloadmeta.SanitizeName(w.Name) + ".log",
		Mounts:     mounts,
	}
	if res := criLinuxResources(w); res != nil {
		cfg.Linux = &runtimeapi.LinuxContainerConfig{Resources: res}
	}
	return cfg, nil
}

func (p *Provider) Deploy(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload) error {
	clients, err := dialCRI(ctx, containerdSocket())
	if err != nil {
		return err
	}
	defer clients.Close()

	ref := workloadmeta.NormalizeImageRef(w.Run.Artifact.Image)
	if ref == "" {
		return oopsx.B("runtime", "containerd").Errorf("workload %q: run.artifact.image is required", w.Name)
	}
	if ready, err := p.prepareExistingCRIWorkload(ctx, clients.runtime, meta, w); err != nil || ready {
		return err
	}

	sandboxCfg := criSandboxConfig(p, meta, w)
	if err := os.MkdirAll(sandboxCfg.LogDirectory, 0o755); err != nil {
		return oopsx.B("runtime", "containerd").Wrapf(err, "create containerd CRI log dir")
	}

	if _, err := clients.image.PullImage(ctx, &runtimeapi.PullImageRequest{
		Image:         &runtimeapi.ImageSpec{Image: ref},
		SandboxConfig: sandboxCfg,
	}); err != nil {
		return oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI pull %q", ref)
	}

	sandbox, err := clients.runtime.RunPodSandbox(ctx, &runtimeapi.RunPodSandboxRequest{Config: sandboxCfg})
	if err != nil {
		return oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI run sandbox")
	}
	sandboxID := sandbox.GetPodSandboxId()

	containerCfg, err := criContainerConfig(p, ref, meta, w)
	if err != nil {
		cleanupCRIWorkload(ctx, clients.runtime, "", sandboxID)
		return oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI container config")
	}

	created, err := clients.runtime.CreateContainer(ctx, &runtimeapi.CreateContainerRequest{
		PodSandboxId:  sandboxID,
		Config:        containerCfg,
		SandboxConfig: sandboxCfg,
	})
	if err != nil {
		cleanupCRIWorkload(ctx, clients.runtime, "", sandboxID)
		return oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI create container")
	}
	containerID := created.GetContainerId()

	if _, err := clients.runtime.StartContainer(ctx, &runtimeapi.StartContainerRequest{ContainerId: containerID}); err != nil {
		cleanupCRIWorkload(ctx, clients.runtime, containerID, sandboxID)
		return oopsx.B("runtime", "containerd").Wrapf(err, "containerd CRI start container")
	}

	ip, err := waitSandboxIP(ctx, clients.runtime, sandboxID)
	if err != nil {
		cleanupCRIWorkload(ctx, clients.runtime, containerID, sandboxID)
		return err
	}

	if p.dns != nil {
		if err := p.dns.UpsertWorkloadA(ctx, meta.Namespace, w.Name, ip); err != nil {
			cleanupCRIWorkload(ctx, clients.runtime, containerID, sandboxID)
			return err
		}
	}

	p.logger.Info("containerd workload running", "sandbox", sandboxID, "container", containerID, "workload", w.Name, "ip", ip)
	return nil
}
