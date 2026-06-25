package runconfig_test

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
)

func TestEnv(t *testing.T) {
	got := runconfig.Env(list.NewList(
		deployv1.EnvVar{Name: " PORT ", Value: "8080"},
		deployv1.EnvVar{Name: "", Value: "skip"},
		deployv1.EnvVar{Name: "EMPTY"},
	))
	want := []string{"PORT=8080", "EMPTY="}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("Env() = %#v, want %#v", got.Values(), want)
	}
}

func TestCommandArgs(t *testing.T) {
	got := runconfig.CommandArgs(deployv1.RunSpec{
		Exec: deployv1.ExecSpec{
			Command: []string{"/bin/server"},
			Args:    []string{"--port", "8080"},
		},
	})
	want := []string{"/bin/server", "--port", "8080"}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("CommandArgs() = %#v, want %#v", got.Values(), want)
	}
}

func TestProcessCommand(t *testing.T) {
	exe, args, ok := runconfig.ProcessCommand(deployv1.RunSpec{
		Exec: deployv1.ExecSpec{
			Command: []string{"/bin/server", "serve"},
			Args:    []string{"--port", "8080"},
		},
	})
	if !ok || exe != "/bin/server" || !slices.Equal(args.Values(), []string{"serve", "--port", "8080"}) {
		t.Fatalf("ProcessCommand() = %q %#v %v", exe, args.Values(), ok)
	}

	exe, args, ok = runconfig.ProcessCommand(deployv1.RunSpec{
		Artifact: deployv1.ArtifactSpec{Path: "/opt/app/api"},
		Exec:     deployv1.ExecSpec{Args: []string{"--port", "8080"}},
	})
	if !ok || exe != "/opt/app/api" || !slices.Equal(args.Values(), []string{"--port", "8080"}) {
		t.Fatalf("ProcessCommand(path) = %q %#v %v", exe, args.Values(), ok)
	}
}

func TestArtifactSummary(t *testing.T) {
	got := runconfig.ArtifactSummary(deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "nginx", Path: "/ignored"}})
	if got != "nginx" {
		t.Fatalf("ArtifactSummary(image) = %q", got)
	}
	got = runconfig.ArtifactSummary(deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Path: "/opt/app/api"}})
	if got != "/opt/app/api" {
		t.Fatalf("ArtifactSummary(path) = %q", got)
	}
	got = runconfig.ArtifactSummary(deployv1.RunSpec{Exec: deployv1.ExecSpec{Command: []string{"/opt/app/worker"}}})
	if got != "/opt/app/worker" {
		t.Fatalf("ArtifactSummary(command) = %q", got)
	}
}

func TestNanoCPUs(t *testing.T) {
	if got := runconfig.NanoCPUs(1500); got != 1_500_000_000 {
		t.Fatalf("NanoCPUs(1500) = %d", got)
	}
	if got := runconfig.NanoCPUs(math.MaxInt64); got != math.MaxInt64 {
		t.Fatalf("NanoCPUs(MaxInt64) = %d", got)
	}
}

func TestCFSQuota(t *testing.T) {
	quota, period := runconfig.CFSQuota(250)
	if quota != 25_000 || period != 100_000 {
		t.Fatalf("CFSQuota(250) = (%d, %d), want (25000, 100000)", quota, period)
	}
}

func TestLocalMounts(t *testing.T) {
	root := t.TempDir()
	meta := deployv1.Metadata{Name: "Demo_App", Namespace: "Prod"}
	workload := deployv1.Workload{
		Name: "api",
		Mounts: []deployv1.Mount{
			{Volume: deployv1.VolumeRef{Name: "Redis Data"}, Target: " /data ", ReadOnly: true},
		},
	}

	got, err := runconfig.LocalMounts(root, meta, workload)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != 1 {
		t.Fatalf("mount count = %d", got.Len())
	}
	mount := got.Values()[0]
	if mount.VolumeName != "orch-prod-demo_app-redis-data" {
		t.Fatalf("volume name = %q", mount.VolumeName)
	}
	if mount.Target != "/data" || !mount.ReadOnly {
		t.Fatalf("target/readOnly = %q/%v", mount.Target, mount.ReadOnly)
	}
	wantSource := filepath.Join(root, "volumes", "prod", "demo_app", "redis-data")
	if mount.SourcePath != wantSource {
		t.Fatalf("source = %q, want %q", mount.SourcePath, wantSource)
	}
}

func TestLocalMountsRejectsRelativeTarget(t *testing.T) {
	_, err := runconfig.LocalMounts("", deployv1.Metadata{Name: "demo"}, deployv1.Workload{
		Name:   "api",
		Mounts: []deployv1.Mount{{Volume: deployv1.VolumeRef{Name: "data"}, Target: "data"}},
	})
	if err == nil || !strings.Contains(err.Error(), "target must be absolute") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsureLocalMountSources(t *testing.T) {
	root := t.TempDir()
	mounts, err := runconfig.LocalMounts(root, deployv1.Metadata{Name: "demo"}, deployv1.Workload{
		Name:   "api",
		Mounts: []deployv1.Mount{{Volume: deployv1.VolumeRef{Name: "data"}, Target: "/data"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runconfig.EnsureLocalMountSources(mounts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mounts.Values()[0].SourcePath); err != nil {
		t.Fatalf("source dir stat: %v", err)
	}
}
