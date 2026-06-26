package docker

import (
	"context"
	"io"
	"log/slog"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
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
