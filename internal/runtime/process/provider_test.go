package process_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/process"
	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
)

func TestProviderDeployStop(t *testing.T) {
	if os.Getenv("ORCH_PROCESS_HELPER") == "1" {
		time.Sleep(time.Minute)
		return
	}

	provider := process.NewProviderWithRoot(slog.New(slog.DiscardHandler), nil, t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	meta := deployv1.Metadata{Name: "demo", Namespace: "default"}
	workload := deployv1.Workload{
		Name:    "worker",
		Runtime: deployv1.RuntimeProcess,
		Run: deployv1.RunSpec{
			Exec: deployv1.ExecSpec{
				Command: []string{exe, "-test.run=TestProviderDeployStop"},
			},
			Env: []deployv1.EnvVar{
				{Name: "ORCH_PROCESS_HELPER", Value: "1"},
			},
			Options: deployv1.RunOptions{
				Process: &deployv1.ProcessOptions{GracefulStopTimeout: "10ms"},
			},
		},
	}

	if err := provider.Deploy(context.Background(), meta, workload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(provider.StatePath(meta, workload.Name)); err != nil {
		t.Fatalf("state file: %v", err)
	}
	if err := provider.Stop(context.Background(), meta, workload.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(provider.StatePath(meta, workload.Name)); !os.IsNotExist(err) {
		t.Fatalf("state file after stop error = %v, want not exist", err)
	}
}

func TestProviderLogsUseConfiguredPaths(t *testing.T) {
	if os.Getenv("ORCH_PROCESS_LOG_HELPER") == "1" {
		runProcessLogHelper(t)
		return
	}

	provider := process.NewProviderWithRoot(slog.New(slog.DiscardHandler), nil, t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	meta := deployv1.Metadata{Name: "demo", Namespace: "default"}
	workload := deployv1.Workload{
		Name:    "logger",
		Runtime: deployv1.RuntimeProcess,
		Run: deployv1.RunSpec{
			Exec: deployv1.ExecSpec{
				Command: []string{exe, "-test.run=TestProviderLogsUseConfiguredPaths"},
			},
			Env: []deployv1.EnvVar{
				{Name: "ORCH_PROCESS_LOG_HELPER", Value: "1"},
			},
			Options: deployv1.RunOptions{
				Process: &deployv1.ProcessOptions{
					GracefulStopTimeout: "10ms",
					StdoutPath:          filepath.Join(logDir, "custom.stdout.log"),
					StderrPath:          filepath.Join(logDir, "custom.stderr.log"),
				},
			},
		},
	}

	if err := provider.Deploy(context.Background(), meta, workload); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Stop(context.Background(), meta, workload.Name); err != nil {
			t.Logf("stop process provider: %v", err)
		}
	})

	logs := waitProcessLogs(t, provider, meta, workload.Name)
	if !strings.Contains(logs.Content, "process stdout ready") {
		t.Fatalf("stdout log missing: %q", logs.Content)
	}
	if !strings.Contains(logs.Content, "process stderr ready") {
		t.Fatalf("stderr log missing: %q", logs.Content)
	}
	if !strings.Contains(logs.Source, logDir) {
		t.Fatalf("source = %q, want custom log dir", logs.Source)
	}
}

func runProcessLogHelper(t *testing.T) {
	t.Helper()
	if _, err := fmt.Println("process stdout ready"); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stderr, "process stderr ready"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}

func waitProcessLogs(t *testing.T, provider *process.Provider, meta deployv1.Metadata, workloadName string) runtimeLogResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last runtimeLogResult
	for time.Now().Before(deadline) {
		got, err := provider.Logs(context.Background(), meta, workloadName, runtimeinfo.LogOptions{Tail: 20})
		if err == nil {
			last = runtimeLogResult{Content: got.Content, Source: got.Source}
			if strings.Contains(got.Content, "process stdout ready") && strings.Contains(got.Content, "process stderr ready") {
				return last
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

type runtimeLogResult struct {
	Content string
	Source  string
}

func TestProviderRejectsMounts(t *testing.T) {
	provider := process.NewProviderWithRoot(slog.New(slog.DiscardHandler), nil, t.TempDir())
	meta := deployv1.Metadata{Name: "demo", Namespace: "default"}
	workload := deployv1.Workload{
		Name:    "worker",
		Runtime: deployv1.RuntimeProcess,
		Run:     deployv1.RunSpec{Exec: deployv1.ExecSpec{Command: []string{"echo"}}},
		Mounts:  []deployv1.Mount{{Volume: deployv1.VolumeRef{Name: "data"}, Target: "/data"}},
	}

	err := provider.Deploy(context.Background(), meta, workload)
	if err == nil || !strings.Contains(err.Error(), "does not support workload mounts") {
		t.Fatalf("err = %v", err)
	}
}
