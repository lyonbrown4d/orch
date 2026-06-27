package composeimport_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/orch/internal/deploy/composeimport"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestLoadComposeFile_mapsServices(t *testing.T) {
	t.Parallel()
	app := loadComposeFixture(t, `
name: orch-test
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    depends_on:
      - db
  db:
    image: postgres:15
`)

	if err := app.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(app.Workloads) != 2 {
		t.Fatalf("workloads: got %d", len(app.Workloads))
	}
	web := requireWorkload(t, app, "web")
	if web.Run.Artifact.Image != "nginx:alpine" {
		t.Fatalf("image %q", web.Run.Artifact.Image)
	}
	if len(web.Endpoints) != 1 || web.Endpoints[0].Port != 80 || web.Endpoints[0].HostPort != 8080 {
		t.Fatalf("endpoints %+v", web.Endpoints)
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0].Name != "db" {
		t.Fatalf("dependsOn %+v", web.DependsOn)
	}
}

func loadComposeFixture(t *testing.T, content string) *deployv1.App {
	t.Helper()
	path := writeComposeFixture(t, content)
	res, err := composeimport.LoadComposeFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return res.App
}

func writeComposeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireWorkload(t *testing.T, app *deployv1.App, name string) *deployv1.Workload {
	t.Helper()
	for i := range app.Workloads {
		if app.Workloads[i].Name == name {
			return &app.Workloads[i]
		}
	}
	t.Fatalf("missing workload %q", name)
	return nil
}
