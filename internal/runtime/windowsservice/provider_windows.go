//go:build windows

package windowsservice

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
	"golang.org/x/sys/windows"
	winsvc "golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type deployServiceSpec struct {
	exe         string
	args        []string
	name        string
	displayName string
	startType   uint32
}

var errServiceStillRunning = errors.New("windows service still running")

func (p *Provider) Deploy(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload) error {
	if err := ctx.Err(); err != nil {
		return oopsx.B("runtime", "windows-service").Wrapf(err, "deploy context")
	}
	if err := runconfig.RejectUnsupportedMounts(deployv1.RuntimeWindowsService, w); err != nil {
		return err
	}
	spec, err := p.deployServiceSpec(meta, w)
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return oopsx.B("runtime", "windows-service").Wrapf(err, "connect service manager")
	}
	defer p.closeManager(manager)

	if absentErr := p.ensureServiceAbsent(manager, spec.name); absentErr != nil {
		return absentErr
	}
	service, err := p.createService(manager, spec)
	if err != nil {
		return err
	}
	defer p.closeService(service, spec.name)

	if err := p.startService(ctx, meta, w, service, spec); err != nil {
		return err
	}
	p.logger.Info("windows service workload running", "workload", w.Name, "service", spec.name)
	return nil
}

func (p *Provider) deployServiceSpec(meta deployv1.Metadata, w deployv1.Workload) (deployServiceSpec, error) {
	exe, args, ok := runconfig.ProcessCommand(w.Run)
	if !ok {
		return deployServiceSpec{}, oopsx.B("runtime", "windows-service").Errorf("workload %q: run.exec.command or run.artifact.path is required", w.Name)
	}
	startType, err := serviceStartType(w)
	if err != nil {
		return deployServiceSpec{}, err
	}
	return deployServiceSpec{
		exe:         exe,
		args:        args.Values(),
		name:        p.workloadServiceName(meta, w),
		displayName: p.workloadDisplayName(meta, w),
		startType:   startType,
	}, nil
}

func (p *Provider) ensureServiceAbsent(manager *mgr.Mgr, serviceName string) error {
	existing, err := manager.OpenService(serviceName)
	if err == nil {
		p.closeService(existing, serviceName)
		return oopsx.B("runtime", "windows-service").Errorf("windows service %q already exists", serviceName)
	}
	return nil
}

func (p *Provider) createService(manager *mgr.Mgr, spec deployServiceSpec) (*mgr.Service, error) {
	service, err := manager.CreateService(spec.name, spec.exe, mgr.Config{
		DisplayName:  spec.displayName,
		StartType:    spec.startType,
		ErrorControl: mgr.ErrorNormal,
		Description:  "orch managed workload",
	}, spec.args...)
	if err != nil {
		return nil, oopsx.B("runtime", "windows-service").Wrapf(err, "create service %s", spec.name)
	}
	return service, nil
}

func (p *Provider) startService(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload, service *mgr.Service, spec deployServiceSpec) error {
	if err := service.Start(); err != nil {
		p.deleteService(service, spec.name)
		return oopsx.B("runtime", "windows-service").Wrapf(err, "start service %s", spec.name)
	}
	st := state{
		ServiceName: spec.name,
		Metadata:    meta,
		Workload:    w.Name,
		Runtime:     w.Runtime,
		Artifact:    runconfig.ArtifactSummary(w.Run),
		StartedAt:   time.Now().UTC(),
	}
	if err := p.writeState(meta, w.Name, st); err != nil {
		p.deleteService(service, spec.name)
		return err
	}
	if err := p.upsertWorkloadDNS(ctx, meta, w.Name); err != nil {
		p.deleteService(service, spec.name)
		p.cleanupRemoveState(meta, w.Name)
		return err
	}
	return nil
}

func (p *Provider) upsertWorkloadDNS(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	if p.dns == nil {
		return nil
	}
	err := p.dns.UpsertWorkloadA(ctx, meta.Namespace, workloadName, p.dns.WorkloadAdvertiseAddress("127.0.0.1"))
	if err != nil {
		return oopsx.B("runtime", "dns").Wrapf(err, "upsert workload DNS")
	}
	return nil
}

func (p *Provider) Stop(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	if err := ctx.Err(); err != nil {
		return oopsx.B("runtime", "windows-service").Wrapf(err, "stop context")
	}
	serviceName, err := p.stopServiceName(meta, workloadName)
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return oopsx.B("runtime", "windows-service").Wrapf(err, "connect service manager")
	}
	defer p.closeManager(manager)

	service, err := manager.OpenService(serviceName)
	if err != nil {
		return p.cleanupMissingService(ctx, meta, workloadName)
	}
	defer p.closeService(service, serviceName)

	if err := p.stopOpenedService(ctx, service, serviceName); err != nil {
		return err
	}
	if err := p.removeWorkloadDNS(ctx, meta, workloadName); err != nil {
		return err
	}
	if err := p.removeState(meta, workloadName); err != nil {
		return err
	}
	p.logger.Info("windows service workload stopped", "workload", workloadName, "service", serviceName)
	return nil
}

func (p *Provider) stopServiceName(meta deployv1.Metadata, workloadName string) (string, error) {
	serviceName := defaultServiceName(meta, workloadName)
	st, err := p.readState(meta, workloadName)
	if err == nil && strings.TrimSpace(st.ServiceName) != "" {
		return st.ServiceName, nil
	}
	if err != nil && !windowsStateNotFound(err) {
		return "", err
	}
	return serviceName, nil
}

func windowsStateNotFound(err error) bool {
	return errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND)
}

func (p *Provider) cleanupMissingService(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	if p.dns != nil {
		p.cleanupRemoveDNS(ctx, meta, workloadName)
	}
	return p.removeState(meta, workloadName)
}

func (p *Provider) stopOpenedService(ctx context.Context, service *mgr.Service, serviceName string) error {
	if p.serviceRunning(service) {
		if _, err := service.Control(winsvc.Stop); err != nil {
			p.logger.Warn("windows service stop control", "service", serviceName, "error", err)
		}
		waitServiceStopped(ctx, service, 10*time.Second)
	}
	if err := service.Delete(); err != nil {
		return oopsx.B("runtime", "windows-service").Wrapf(err, "delete service %s", serviceName)
	}
	return nil
}

func (p *Provider) serviceRunning(service *mgr.Service) bool {
	status, err := service.Query()
	return err == nil && status.State != winsvc.Stopped
}

func (p *Provider) removeWorkloadDNS(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	if p.dns == nil {
		return nil
	}
	if err := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); err != nil {
		return oopsx.B("runtime", "dns").Wrapf(err, "remove workload DNS")
	}
	return nil
}

func (p *Provider) closeManager(manager *mgr.Mgr) {
	if err := manager.Disconnect(); err != nil {
		p.logger.Warn("windows service manager disconnect", "error", err)
	}
}

func (p *Provider) closeService(service *mgr.Service, serviceName string) {
	if err := service.Close(); err != nil {
		p.logger.Warn("windows service close", "service", serviceName, "error", err)
	}
}

func (p *Provider) deleteService(service *mgr.Service, serviceName string) {
	if err := service.Delete(); err != nil {
		p.logger.Warn("windows service delete cleanup", "service", serviceName, "error", err)
	}
}

func (p *Provider) cleanupRemoveState(meta deployv1.Metadata, workloadName string) {
	if err := p.removeState(meta, workloadName); err != nil {
		p.logger.Warn("windows service state cleanup", "workload", workloadName, "error", err)
	}
}

func (p *Provider) cleanupRemoveDNS(ctx context.Context, meta deployv1.Metadata, workloadName string) {
	if err := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); err != nil {
		p.logger.Warn("windows service DNS cleanup", "workload", workloadName, "error", err)
	}
}

func serviceStartType(w deployv1.Workload) (uint32, error) {
	startType := "manual"
	if w.Run.Options.WindowsService != nil {
		if v := strings.TrimSpace(strings.ToLower(w.Run.Options.WindowsService.StartType)); v != "" {
			startType = v
		}
	}
	switch startType {
	case "manual", "demand":
		return mgr.StartManual, nil
	case "automatic", "auto":
		return mgr.StartAutomatic, nil
	case "disabled":
		return mgr.StartDisabled, nil
	default:
		return 0, oopsx.B("runtime", "windows-service").Errorf("invalid windowsService.startType %q", startType)
	}
}

func waitServiceStopped(ctx context.Context, service *mgr.Service, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := backoff.Retry(ctx, func() (struct{}, error) {
		status, err := service.Query()
		if err != nil {
			return struct{}{}, backoff.Permanent(err)
		}
		if status.State == winsvc.Stopped {
			return struct{}{}, nil
		}
		return struct{}{}, errServiceStillRunning
	}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond))); err != nil {
		return
	}
}
