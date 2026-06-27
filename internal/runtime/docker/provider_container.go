package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/cenkalti/backoff/v5"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	dockermount "github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (p *Provider) deployWorkloadContainer(ctx context.Context, cli *client.Client, meta deployv1.Metadata, w deployv1.Workload, ref string) error {
	name := workloadmeta.OrchContainerName(meta, w.Name)
	ctrCfg := dockerContainerConfig(meta, w, ref)
	hostCfg, err := p.dockerHostConfig(meta, w)
	if err != nil {
		return err
	}

	_, err = backoff.Retry(ctx, func() (struct{}, error) {
		return p.deployWorkloadContainerOnce(ctx, cli, meta, w, name, ctrCfg, hostCfg)
	}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(10))
	if err != nil {
		return oopsx.B("runtime", "docker").Wrapf(err, "docker deploy workload %q", w.Name)
	}
	return nil
}

func dockerContainerConfig(meta deployv1.Metadata, w deployv1.Workload, ref string) *container.Config {
	cfg := &container.Config{
		Image:      ref,
		Entrypoint: w.Run.Exec.Command,
		Cmd:        w.Run.Exec.Args,
		Env:        runconfig.Env(w.EnvList()).Values(),
		WorkingDir: strings.TrimSpace(w.Run.Cwd),
		Labels:     ContainerLabels(meta, w),
	}
	ApplyEndpointExposes(cfg, w)
	return cfg
}

func (p *Provider) dockerHostConfig(meta deployv1.Metadata, w deployv1.Workload) (*container.HostConfig, error) {
	hostCfg := &container.HostConfig{}
	applyDockerResources(hostCfg, w)
	applyDockerOptions(hostCfg, w)
	ApplyEndpointPortBindings(hostCfg, w)
	ApplyWorkloadDNS(hostCfg, p.dns, meta.Namespace)
	localMounts, err := runconfig.LocalMounts("", meta, w)
	if err != nil {
		return nil, oopsx.B("runtime", "docker").Wrapf(err, "resolve local mounts")
	}
	ApplyLocalMounts(hostCfg, localMounts)
	return hostCfg, nil
}

func dockerEndpointPort(endpoint deployv1.Endpoint) nat.Port {
	proto := "tcp"
	if endpoint.Protocol == deployv1.ProtoUDP {
		proto = "udp"
	}
	return nat.Port(strconv.Itoa(endpoint.Port) + "/" + proto)
}

// ApplyEndpointExposes records workload endpoints in Docker container metadata.
func ApplyEndpointExposes(cfg *container.Config, w deployv1.Workload) {
	if cfg == nil || len(w.Endpoints) == 0 {
		return
	}
	if cfg.ExposedPorts == nil {
		cfg.ExposedPorts = nat.PortSet{}
	}
	for _, endpoint := range w.Endpoints {
		cfg.ExposedPorts[dockerEndpointPort(endpoint)] = struct{}{}
	}
}

// ApplyEndpointPortBindings publishes endpoint ports on the Docker host when hostPort is set.
func ApplyEndpointPortBindings(hostCfg *container.HostConfig, w deployv1.Workload) {
	if hostCfg == nil || len(w.Endpoints) == 0 {
		return
	}
	for _, endpoint := range w.Endpoints {
		if endpoint.HostPort <= 0 {
			continue
		}
		if hostCfg.PortBindings == nil {
			hostCfg.PortBindings = nat.PortMap{}
		}
		port := dockerEndpointPort(endpoint)
		hostCfg.PortBindings[port] = append(hostCfg.PortBindings[port], nat.PortBinding{
			HostIP:   strings.TrimSpace(endpoint.HostIP),
			HostPort: strconv.Itoa(endpoint.HostPort),
		})
	}
}
func applyDockerResources(hostCfg *container.HostConfig, w deployv1.Workload) {
	if w.Resources == nil {
		return
	}
	hostCfg.Memory = w.Resources.MemoryBytes
	hostCfg.NanoCPUs = runconfig.NanoCPUs(w.Resources.CPUMillis)
}

func applyDockerOptions(hostCfg *container.HostConfig, w deployv1.Workload) {
	if w.Run.Options.Docker == nil {
		return
	}
	if mode := strings.TrimSpace(w.Run.Options.Docker.NetworkMode); mode != "" {
		hostCfg.NetworkMode = container.NetworkMode(mode)
	}
	hostCfg.Privileged = w.Run.Options.Docker.Privileged
}

func (p *Provider) deployWorkloadContainerOnce(
	ctx context.Context,
	cli *client.Client,
	meta deployv1.Metadata,
	w deployv1.Workload,
	name string,
	ctrCfg *container.Config,
	hostCfg *container.HostConfig,
) (struct{}, error) {
	containerID, createErr := p.createDockerContainer(ctx, cli, meta, w, name, ctrCfg, hostCfg)
	if createErr != nil {
		return struct{}{}, permanentDockerDeployError(createErr)
	}
	if containerID == "" {
		return struct{}{}, nil
	}
	if err := p.dockerRunAfterCreate(ctx, cli, meta, w, name, containerID); err != nil {
		if isDockerMarkedForRemoval(err) {
			return struct{}{}, err
		}
		return struct{}{}, permanentDockerDeployError(err)
	}
	return struct{}{}, nil
}

func permanentDockerDeployError(err error) error {
	return fmt.Errorf("permanent docker deploy error: %w", backoff.Permanent(err))
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
			return "", oopsx.B("runtime", "docker").Wrapf(err, "docker create conflict %q", name)
		}
		return "", backoff.Permanent(oopsx.B("runtime", "docker").Wrapf(err, "docker create %q", name))
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
