//go:build linux

package systemd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (p *Provider) Deploy(ctx context.Context, meta deployv1.Metadata, w deployv1.Workload) error {
	unitName := p.workloadUnitName(meta, w)
	unitPath := systemdUnitPath(unitName)
	content, err := RenderUnit(meta, w, unitName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "create systemd unit dir")
	}
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "write unit %s", unitName)
	}
	conn, err := p.connect(ctx)
	if err != nil {
		_ = os.Remove(unitPath)
		return oopsx.B("runtime", "systemd").Wrapf(err, "connect systemd dbus")
	}
	defer conn.Close()

	if err := systemdReload(ctx, conn); err != nil {
		_ = os.Remove(unitPath)
		return err
	}
	if err := systemdEnable(ctx, conn, unitPath); err != nil {
		p.cleanupUnit(ctx, unitName, unitPath)
		return err
	}
	if err := systemdStart(ctx, conn, unitName); err != nil {
		p.cleanupUnit(ctx, unitName, unitPath)
		return err
	}

	st := state{
		UnitName:  unitName,
		UnitPath:  unitPath,
		Metadata:  meta,
		Workload:  w.Name,
		Runtime:   w.Runtime,
		Artifact:  runconfig.ArtifactSummary(w.Run),
		StartedAt: time.Now().UTC(),
	}
	if err := p.writeState(meta, w.Name, st); err != nil {
		p.cleanupUnit(ctx, unitName, unitPath)
		return err
	}
	if p.dns != nil {
		if err := p.dns.UpsertWorkloadA(ctx, meta.Namespace, w.Name, p.dns.WorkloadAdvertiseAddress("127.0.0.1")); err != nil {
			p.cleanupUnit(ctx, unitName, unitPath)
			_ = p.removeState(meta, w.Name)
			return oopsx.B("runtime", "dns").Wrapf(err, "upsert workload DNS")
		}
	}
	p.logger.Info("systemd workload running", "workload", w.Name, "unit", unitName)
	return nil
}

func (p *Provider) connect(ctx context.Context) (Connection, error) {
	if p.connector == nil {
		p.connector = NewConnector()
	}
	return p.connector(ctx)
}

func (p *Provider) Stop(ctx context.Context, meta deployv1.Metadata, workloadName string) error {
	unitName := DefaultUnitName(meta, workloadName)
	unitPath := systemdUnitPath(unitName)
	if st, err := p.readState(meta, workloadName); err == nil {
		if strings.TrimSpace(st.UnitName) != "" {
			unitName = st.UnitName
		}
		if strings.TrimSpace(st.UnitPath) != "" {
			unitPath = st.UnitPath
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	conn, err := p.connect(ctx)
	if err != nil {
		p.logger.Warn("systemd dbus connect", "error", err)
	} else {
		defer conn.Close()
		if err := systemdStop(ctx, conn, unitName); err != nil {
			p.logger.Warn("systemd stop unit", "unit", unitName, "error", err)
		}
		if err := systemdDisable(ctx, conn, unitName); err != nil {
			p.logger.Warn("systemd disable unit", "unit", unitName, "error", err)
		}
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return oopsx.B("runtime", "systemd").Wrapf(err, "remove unit %s", unitName)
	}
	if conn != nil {
		if err := systemdReload(ctx, conn); err != nil {
			p.logger.Warn("systemd daemon-reload", "error", err)
		}
	} else if err := p.reloadSystemdWithNewConnection(ctx); err != nil {
		p.logger.Warn("systemd daemon-reload", "error", err)
	}
	if p.dns != nil {
		if err := p.dns.RemoveWorkloadA(ctx, meta.Namespace, workloadName); err != nil {
			return oopsx.B("runtime", "dns").Wrapf(err, "remove workload DNS")
		}
	}
	if err := p.removeState(meta, workloadName); err != nil {
		return err
	}
	p.logger.Info("systemd workload stopped", "workload", workloadName, "unit", unitName)
	return nil
}

func (p *Provider) cleanupUnit(ctx context.Context, unitName, unitPath string) {
	conn, err := p.connect(ctx)
	if err != nil {
		p.logger.Warn("systemd cleanup dbus connect", "error", err)
	} else {
		defer conn.Close()
		if err := systemdStop(ctx, conn, unitName); err != nil {
			p.logger.Warn("systemd cleanup stop unit", "unit", unitName, "error", err)
		}
		if err := systemdDisable(ctx, conn, unitName); err != nil {
			p.logger.Warn("systemd cleanup disable unit", "unit", unitName, "error", err)
		}
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		p.logger.Warn("systemd cleanup remove unit", "unit", unitName, "error", err)
	}
	if conn != nil {
		if err := systemdReload(ctx, conn); err != nil {
			p.logger.Warn("systemd cleanup daemon-reload", "error", err)
		}
	} else if err := p.reloadSystemdWithNewConnection(ctx); err != nil {
		p.logger.Warn("systemd cleanup daemon-reload", "error", err)
	}
}

func systemdStart(ctx context.Context, conn Connection, unitName string) error {
	ch := make(chan string, 1)
	if _, err := conn.StartUnitContext(ctx, unitName, "replace", ch); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "start unit %s", unitName)
	}
	return waitSystemdJob(ctx, ch, "start", unitName)
}

func systemdStop(ctx context.Context, conn Connection, unitName string) error {
	ch := make(chan string, 1)
	if _, err := conn.StopUnitContext(ctx, unitName, "replace", ch); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "stop unit %s", unitName)
	}
	return waitSystemdJob(ctx, ch, "stop", unitName)
}

func systemdEnable(ctx context.Context, conn Connection, unitPath string) error {
	if _, _, err := conn.EnableUnitFilesContext(ctx, []string{unitPath}, false, true); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "enable unit %s", filepath.Base(unitPath))
	}
	return nil
}

func systemdDisable(ctx context.Context, conn Connection, unitName string) error {
	if _, err := conn.DisableUnitFilesContext(ctx, []string{unitName}, false); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "disable unit %s", unitName)
	}
	return nil
}

func systemdReload(ctx context.Context, conn Connection) error {
	if err := conn.ReloadContext(ctx); err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "daemon reload")
	}
	return nil
}

func (p *Provider) reloadSystemdWithNewConnection(ctx context.Context) error {
	conn, err := p.connect(ctx)
	if err != nil {
		return oopsx.B("runtime", "systemd").Wrapf(err, "connect systemd dbus")
	}
	defer conn.Close()
	return systemdReload(ctx, conn)
}

func waitSystemdJob(ctx context.Context, ch <-chan string, action, unitName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-ch:
		if result != "done" {
			return oopsx.B("runtime", "systemd").Errorf("%s unit %s: %s", action, unitName, result)
		}
	}
	return nil
}
