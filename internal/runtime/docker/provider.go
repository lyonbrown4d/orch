package docker

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/cenkalti/backoff/v5"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockermount "github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type Provider struct {
	logger *slog.Logger
	dns    *dnssvc.Service
	kind   deployv1.RuntimeKind

	newClient clientFactory
}

type clientFactory func() (*client.Client, error)

func NewProvider(logger *slog.Logger, dns *dnssvc.Service) *Provider {
	return NewProviderWithKind(logger, dns, deployv1.RuntimeDocker, nil)
}

func NewProviderWithKind(logger *slog.Logger, dns *dnssvc.Service, kind deployv1.RuntimeKind, makeClient clientFactory) *Provider {
	if strings.TrimSpace(string(kind)) == "" {
		kind = deployv1.RuntimeDocker
	}
	if makeClient == nil {
		makeClient = defaultDockerClient
	}
	return &Provider{
		logger:    logger,
		dns:       dns,
		kind:      kind,
		newClient: makeClient,
	}
}

func (p *Provider) Kind() deployv1.RuntimeKind {
	if p.kind == "" {
		return deployv1.RuntimeDocker
	}
	return p.kind
}

func (p *Provider) runtime() string {
	return string(p.Kind())
}

func workloadContainerFilters(meta deployv1.Metadata, workloadName string) filters.Args {
	return filters.NewArgs(
		filters.Arg("label", "orch.io/app="+strings.TrimSpace(meta.Name)),
		filters.Arg("label", "orch.io/namespace="+workloadmeta.NamespaceOrDefault(meta.Namespace)),
		filters.Arg("label", "orch.io/workload="+strings.TrimSpace(workloadName)),
	)
}

func (p *Provider) drainDockerImagePull(ctx context.Context, cli *client.Client, ref string) error {
	pull, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return oopsx.B("runtime", "docker").Wrapf(err, "docker pull %q", ref)
	}
	defer func() {
		if closeErr := pull.Close(); closeErr != nil {
			p.logger.Warn("docker pull reader close", "error", closeErr)
		}
	}()
	if _, copyErr := io.Copy(io.Discard, pull); copyErr != nil {
		return oopsx.B("runtime", "docker").Wrapf(copyErr, "docker pull drain %q", ref)
	}
	return nil
}

func (p *Provider) ensureDockerImage(ctx context.Context, cli *client.Client, ref string) error {
	if err := p.drainDockerImagePull(ctx, cli, ref); err != nil {
		if _, inspectErr := cli.ImageInspect(ctx, ref); inspectErr == nil {
			p.logger.Warn("docker pull failed; using cached local image", "image", ref, "error", err)
			return nil
		}
		return err
	}
	return nil
}

func (p *Provider) prepareExistingDockerContainer(ctx context.Context, cli *client.Client, meta deployv1.Metadata, w deployv1.Workload, name string) (bool, error) {
	inspect, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, oopsx.B("runtime", "docker").Wrapf(err, "docker inspect existing container %q", name)
	}
	labels := map[string]string{}
	if inspect.Config != nil {
		labels = inspect.Config.Labels
	}
	if !WorkloadLabelsMatch(labels, meta, w) {
		return false, oopsx.B("runtime", "docker").Errorf("docker: container %q already exists and is not managed by app %s/%s workload %s",
			name, workloadmeta.NamespaceOrDefault(meta.Namespace), meta.Name, w.Name)
	}
	if inspect.State != nil && inspect.State.Running {
		if err := p.recordDockerWorkloadDNS(ctx, meta, w, name, inspect); err != nil {
			return false, err
		}
		p.logger.Info("docker workload already running", "container", name, "workload", w.Name)
		return true, nil
	}
	if err := cli.ContainerRemove(ctx, inspect.ID, container.RemoveOptions{Force: true}); err != nil {
		return false, oopsx.B("runtime", "docker").Wrapf(err, "docker remove stale container %q", name)
	}
	p.logger.Info("docker stale workload container removed", "container", name, "workload", w.Name)
	return false, nil
}

func (p *Provider) deployWorkloadContainer(ctx context.Context, cli *client.Client, meta deployv1.Metadata, w deployv1.Workload, ref string) error {
	name := workloadmeta.OrchContainerName(meta, w.Name)
	ctrCfg := &container.Config{
		Image:      ref,
		Entrypoint: w.Run.Exec.Command,
		Cmd:        w.Run.Exec.Args,
		Env:        runconfig.Env(w.EnvList()).Values(),
		WorkingDir: strings.TrimSpace(w.Run.Cwd),
		Labels:     ContainerLabels(meta, w),
	}

	hostCfg := &container.HostConfig{}
	if w.Resources != nil {
		hostCfg.Memory = w.Resources.MemoryBytes
		hostCfg.NanoCPUs = runconfig.NanoCPUs(w.Resources.CPUMillis)
	}
	if w.Run.Options.Docker != nil {
		if m := strings.TrimSpace(w.Run.Options.Docker.NetworkMode); m != "" {
			hostCfg.NetworkMode = container.NetworkMode(m)
		}
		hostCfg.Privileged = w.Run.Options.Docker.Privileged
	}
	ApplyWorkloadDNS(hostCfg, p.dns, meta.Namespace)
	localMounts, err := runconfig.LocalMounts("", meta, w)
	if err != nil {
		return err
	}
	ApplyLocalMounts(hostCfg, localMounts)

	_, err = backoff.Retry(ctx, func() (struct{}, error) {
		containerID, err := p.createDockerContainer(ctx, cli, meta, w, name, ctrCfg, hostCfg)
		if err != nil {
			return struct{}{}, backoff.Permanent(err)
		}
		if containerID == "" {
			return struct{}{}, nil
		}
		if err := p.dockerRunAfterCreate(ctx, cli, meta, w, name, containerID); err != nil {
			if isDockerMarkedForRemoval(err) {
				return struct{}{}, err
			}
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(10))
	return err
}

// ApplyLocalMounts injects local runtime volumes into Docker-compatible host config.
func ApplyLocalMounts(hostCfg *container.HostConfig, mounts *list.List[runconfig.Mount]) {
	if hostCfg == nil || mounts == nil {
		return
	}
	mounts.Range(func(_ int, m runconfig.Mount) bool {
		hostCfg.Mounts = append(hostCfg.Mounts, dockermount.Mount{
			Type:     dockermount.TypeVolume,
			Source:   m.VolumeName,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
		return true
	})
}

func (p *Provider) createDockerContainer(
	ctx context.Context,
	cli *client.Client,
	meta deployv1.Metadata,
	w deployv1.Workload,
	name string,
	ctrCfg *container.Config,
	hostCfg *container.HostConfig,
) (string, error) {
	createResp, err := cli.ContainerCreate(ctx, ctrCfg, hostCfg, nil, nil, name)
	if err == nil {
		return createResp.ID, nil
	}
	if !cerrdefs.IsConflict(err) {
		return "", oopsx.B("runtime", "docker").Wrapf(err, "docker create %q", name)
	}
	return p.createDockerContainerAfterConflict(ctx, cli, meta, w, name, ctrCfg, hostCfg)
}

func (p *Provider) createDockerContainerAfterConflict(
	ctx context.Context,
	cli *client.Client,
	meta deployv1.Metadata,
	w deployv1.Workload,
	name string,
	ctrCfg *container.Config,
	hostCfg *container.HostConfig,
) (string, error) {
	containerID, err := backoff.Retry(ctx, func() (string, error) {
		ready, err := p.prepareExistingDockerContainer(ctx, cli, meta, w, name)
		if err != nil {
			return "", backoff.Permanent(err)
		}
		if ready {
			return "", nil
		}
		createResp, err := cli.ContainerCreate(ctx, ctrCfg, hostCfg, nil, nil, name)
		if err == nil {
			return createResp.ID, nil
		}
		if cerrdefs.IsConflict(err) {
			return "", err
		}
		return "", backoff.Permanent(err)
	}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(10))
	if err != nil {
		return "", oopsx.B("runtime", "docker").Wrapf(err, "docker create %q", name)
	}
	return containerID, nil
}

func isDockerMarkedForRemoval(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "marked for removal")
}
func (p *Provider) dockerRunAfterCreate(ctx context.Context, cli *client.Client, meta deployv1.Metadata, w deployv1.Workload, name, containerID string) error {
	removeFailed := func(stage string, cleanupErr error) {
		if rmErr := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); rmErr != nil {
			p.logger.Warn("docker: remove container after failure", "stage", stage, "remove_error", rmErr, "cause", cleanupErr)
		}
	}

	if startErr := cli.ContainerStart(ctx, containerID, container.StartOptions{}); startErr != nil {
		removeFailed("start", startErr)
		return oopsx.B("runtime", "docker").Wrapf(startErr, "docker start %q", containerID)
	}

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		removeFailed("inspect", err)
		return oopsx.B("runtime", "docker").Wrapf(err, "docker inspect after start")
	}
	if err := p.recordDockerWorkloadDNS(ctx, meta, w, name, inspect); err != nil {
		removeFailed("record_dns", err)
		return err
	}

	p.logger.Info("docker workload running", "container", name, "workload", w.Name)
	return nil
}

func (p *Provider) Deploy(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload) error {
	cli, err := p.newDockerClient()
	if err != nil {
		return err
	}
	defer p.closeDockerClient(cli)

	ref := workloadmeta.NormalizeImageRef(w.Run.Artifact.Image)
	if ref == "" {
		return oopsx.B("runtime", "docker").Errorf("docker: workload %q: run.artifact.image is required", w.Name)
	}

	if pullErr := p.ensureDockerImage(ctx, cli, ref); pullErr != nil {
		return pullErr
	}
	return p.deployWorkloadContainer(ctx, cli, meta, w, ref)
}

func (p *Provider) Stop(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	cli, err := p.newDockerClient()
	if err != nil {
		return err
	}
	defer p.closeDockerClient(cli)

	ns := workloadmeta.NamespaceOrDefault(meta.Namespace)
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: workloadContainerFilters(meta, workloadName)})
	if err != nil {
		return oopsx.B("runtime", "docker").Wrapf(err, "docker list containers")
	}
	if len(containers) == 0 {
		if rmErr := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); rmErr != nil {
			p.logger.Warn("dns remove workload record", "error", rmErr)
		}
		p.logger.Debug("docker stop: no container for workload", "workload", workloadName, "namespace", ns)
		return nil
	}
	id := containers[0].ID
	if err := cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return oopsx.B("runtime", "docker").Wrapf(err, "docker remove container")
	}
	if err := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); err != nil {
		return oopsx.B("runtime", "dns").Wrapf(err, "remove workload DNS")
	}
	p.logger.Info("docker workload stopped", "workload", workloadName, "container", id)
	return nil
}
