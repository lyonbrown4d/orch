//go:build linux

package systemd

import (
	"context"
	"testing"

	"github.com/coreos/go-systemd/v22/dbus"
)

type recordingConnection struct {
	disabled []string
}

func (c *recordingConnection) Close() {}

func (c *recordingConnection) ReloadContext(context.Context) error { return nil }

func (c *recordingConnection) StartUnitContext(context.Context, string, string, chan<- string) (int, error) {
	return 0, nil
}

func (c *recordingConnection) StopUnitContext(context.Context, string, string, chan<- string) (int, error) {
	return 0, nil
}

func (c *recordingConnection) EnableUnitFilesContext(context.Context, []string, bool, bool) (bool, []dbus.EnableUnitFileChange, error) {
	return false, nil, nil
}

func (c *recordingConnection) DisableUnitFilesContext(_ context.Context, files []string, _ bool) ([]dbus.DisableUnitFileChange, error) {
	c.disabled = append([]string(nil), files...)
	return nil, nil
}

func (c *recordingConnection) ListUnitsByNamesContext(context.Context, []string) ([]dbus.UnitStatus, error) {
	return nil, nil
}

func TestSystemdDisableUsesUnitName(t *testing.T) {
	t.Parallel()

	conn := &recordingConnection{}
	if err := systemdDisable(context.Background(), conn, "orch-default-demo-api.service"); err != nil {
		t.Fatal(err)
	}
	if len(conn.disabled) != 1 || conn.disabled[0] != "orch-default-demo-api.service" {
		t.Fatalf("disabled files = %#v", conn.disabled)
	}
}
