package process

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const defaultStopTimeout = 5 * time.Second

var errProcessStillRunning = errors.New("process still running")

type Provider struct {
	logger *slog.Logger
	dns    *dnssvc.Service
	root   string
}

func NewProvider(logger *slog.Logger, dns *dnssvc.Service) *Provider {
	return NewProviderWithRoot(logger, dns, filepath.Join(config.DefaultDataRoot(), "runtime", "process"))
}

// NewProviderWithRoot creates a process provider using an explicit runtime root.
func NewProviderWithRoot(logger *slog.Logger, dns *dnssvc.Service, root string) *Provider {
	return &Provider{
		logger: logger,
		dns:    dns,
		root:   root,
	}
}

func (p *Provider) Kind() deployv1.RuntimeKind {
	return deployv1.RuntimeProcess
}

func (p *Provider) Deploy(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload) error {
	exe, args, ok := runconfig.ProcessCommand(w.Run)
	if !ok {
		return oopsx.B("runtime", "process").Errorf("workload %q: run.exec.command or run.artifact.path is required", w.Name)
	}
	if err := runconfig.RejectUnsupportedMounts(deployv1.RuntimeProcess, w); err != nil {
		return err
	}
	if err := p.ensureNoLiveState(meta, w.Name); err != nil {
		return err
	}

	stdoutPath, stderrPath := p.logPaths(meta, w)
	stdout, stderr, closeLogs, err := p.openLogFiles(stdoutPath, stderrPath)
	if err != nil {
		return err
	}

	cmd := &exec.Cmd{
		Path: exe,
		Args: append([]string{exe}, args.Values()...),
	}
	cmd.Env = append(os.Environ(), runconfig.Env(w.EnvList()).Values()...)
	cmd.Dir = strings.TrimSpace(w.Run.Cwd)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		closeLogs()
		return oopsx.B("runtime", "process").Wrapf(err, "process start %q", exe)
	}

	st := state{
		PID:        cmd.Process.Pid,
		Metadata:   meta,
		Workload:   w.Name,
		Runtime:    w.Runtime,
		Artifact:   runconfig.ArtifactSummary(w.Run),
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
		StopAfter:  processStopTimeout(w.Run),
		StartedAt:  time.Now().UTC(),
	}
	if err := p.writeState(meta, w.Name, st); err != nil {
		p.killProcess(cmd.Process, w.Name)
		closeLogs()
		return err
	}

	if p.dns != nil {
		if err := p.dns.UpsertWorkloadA(ctx, meta.Namespace, w.Name, p.dns.WorkloadAdvertiseAddress("127.0.0.1")); err != nil {
			p.killProcess(cmd.Process, w.Name)
			closeLogs()
			p.cleanupRemoveState(meta, w.Name)
			return oopsx.B("runtime", "dns").Wrapf(err, "upsert workload DNS")
		}
	}

	go p.waitProcess(context.WithoutCancel(ctx), meta, w.Name, cmd, closeLogs)
	p.logger.Info("process workload running", "workload", w.Name, "pid", cmd.Process.Pid, "exec", exe)
	return nil
}

func (p *Provider) Stop(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	st, err := p.readState(meta, workloadName)
	if err != nil {
		return p.handleMissingStopState(ctx, meta, workloadName, err)
	}
	p.stopProcess(ctx, st, workloadName)
	if err := p.removeWorkloadDNS(ctx, meta, workloadName); err != nil {
		return err
	}
	if err := p.removeState(meta, workloadName); err != nil {
		return err
	}
	p.logger.Info("process workload stopped", "workload", workloadName, "pid", st.PID)
	return nil
}

func (p *Provider) handleMissingStopState(ctx context.Context, meta deployv1.Metadata, workloadName string, err error) error {
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if p.dns != nil {
		p.cleanupRemoveDNS(ctx, meta, workloadName)
	}
	return nil
}

func (p *Provider) stopProcess(ctx context.Context, st state, workloadName string) {
	proc, err := os.FindProcess(st.PID)
	if err != nil || proc == nil {
		return
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		p.killProcess(proc, workloadName)
		return
	}
	if !waitExit(ctx, st.PID, p.stopTimeout(st)) {
		p.killProcess(proc, workloadName)
	}
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

func (p *Provider) Status(_ context.Context, meta deployv1.Metadata, workloadName string) (runtimeinfo.Status, error) {
	st, err := p.readState(meta, workloadName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runtimeinfo.Status{Name: strings.TrimSpace(workloadName), Runtime: deployv1.RuntimeProcess, Status: "stopped"}, nil
		}
		return runtimeinfo.Status{}, err
	}
	status := "stopped"
	message := "process state exists but pid is not running"
	if st.PID > 0 && processAlive(st.PID) {
		status = "running"
		message = ""
	}
	return runtimeinfo.Status{
		Name:      strings.TrimSpace(workloadName),
		Runtime:   deployv1.RuntimeProcess,
		Status:    status,
		NativeID:  strconv.Itoa(st.PID),
		StartedAt: st.StartedAt,
		UpdatedAt: time.Now().UTC(),
		Message:   message,
	}, nil
}

func (p *Provider) ensureNoLiveState(meta deployv1.Metadata, workloadName string) error {
	st, err := p.readState(meta, workloadName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if st.PID > 0 && processAlive(st.PID) {
		return oopsx.B("runtime", "process").Errorf("process workload %q already has live pid %d", workloadName, st.PID)
	}
	return p.removeState(meta, workloadName)
}

func (p *Provider) waitProcess(ctx context.Context, meta deployv1.Metadata, workloadName string, cmd *exec.Cmd, closeLogs func()) {
	err := cmd.Wait()
	closeLogs()
	if err != nil {
		p.logger.Warn("process workload exited", "workload", workloadName, "pid", cmd.ProcessState.Pid(), "error", err)
	} else {
		p.logger.Info("process workload exited", "workload", workloadName, "pid", cmd.ProcessState.Pid())
	}
	if err := p.removeStateIfPID(meta, workloadName, cmd.ProcessState.Pid()); err != nil {
		p.logger.Warn("process state cleanup failed", "workload", workloadName, "pid", cmd.ProcessState.Pid(), "error", err)
	}
	if p.dns != nil {
		p.cleanupRemoveDNS(ctx, meta, workloadName)
	}
}

func (p *Provider) killProcess(proc *os.Process, workloadName string) {
	if proc == nil {
		return
	}
	if err := proc.Kill(); err != nil {
		p.logger.Warn("process kill failed", "workload", workloadName, "pid", proc.Pid, "error", err)
	}
}

func (p *Provider) cleanupRemoveState(meta deployv1.Metadata, workloadName string) {
	if err := p.removeState(meta, workloadName); err != nil {
		p.logger.Warn("process state cleanup failed", "workload", workloadName, "error", err)
	}
}

func (p *Provider) cleanupRemoveDNS(ctx context.Context, meta deployv1.Metadata, workloadName string) {
	if err := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); err != nil {
		p.logger.Warn("process DNS cleanup failed", "workload", workloadName, "error", err)
	}
}

func (p *Provider) stopTimeout(st state) time.Duration {
	if st.StopAfter != "" {
		if d, err := time.ParseDuration(st.StopAfter); err == nil && d > 0 {
			return d
		}
	}
	return defaultStopTimeout
}

func processStopTimeout(run deployv1.RunSpec) string {
	if run.Options.Process == nil {
		return ""
	}
	return strings.TrimSpace(run.Options.Process.GracefulStopTimeout)
}

func waitExit(ctx context.Context, pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		if !processAlive(pid) {
			return struct{}{}, nil
		}
		return struct{}{}, errProcessStillRunning
	}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)))
	return err == nil || !processAlive(pid)
}
