package api_test

import (
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arcgolabs/httpx"
	"github.com/arcgolabs/httpx/adapter"
	adapterfiber "github.com/arcgolabs/httpx/adapter/fiber"
	"github.com/gofiber/fiber/v3"

	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	deployorch "github.com/lyonbrown4d/orch/internal/deploy/orch"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/metrics"
	"github.com/lyonbrown4d/orch/internal/nodecapacity"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/observability"
	"github.com/lyonbrown4d/orch/internal/placement"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workerapi"
)

const (
	deployFlowNamespace  = "default"
	deployFlowApp        = "e2e-demo"
	deployFlowWorkload   = "worker"
	deployFlowImage      = "busybox"
	deployFlowLocalNode  = "node-a"
	deployFlowRemoteNode = "node-b"
)

type e2eRuntimeProvider struct {
	mu       sync.Mutex
	deployed []deployv1.Workload
}

func (p *e2eRuntimeProvider) Kind() deployv1.RuntimeKind {
	return deployv1.RuntimeDocker
}

func (p *e2eRuntimeProvider) Deploy(_ context.Context, _ deployv1.Metadata, workload deployv1.Workload) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deployed = append(p.deployed, workload)
	return nil
}

func (p *e2eRuntimeProvider) Stop(_ context.Context, _ deployv1.Metadata, _ string) error {
	return nil
}

func (p *e2eRuntimeProvider) deployedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.deployed)
}

type deployFlowHarness struct {
	t            *testing.T
	ctx          context.Context
	client       *apiclient.Client
	workerCh     chan workerapi.DeployWorkloadBody
	stopCh       chan workerapi.StopWorkloadBody
	localRuntime *e2eRuntimeProvider
}

func newDeployFlowHarness(t *testing.T) *deployFlowHarness {
	t.Helper()

	ctx := t.Context()
	logger := slog.New(slog.DiscardHandler)
	workerCh := make(chan workerapi.DeployWorkloadBody, 3)
	stopCh := make(chan workerapi.StopWorkloadBody, 3)
	worker := newDeployFlowWorker(t, workerCh, stopCh)
	cfg := deployFlowConfig(t, worker.URL)
	local := nodeid.Local{Value: deployFlowLocalNode}
	raft := startDeployFlowRaft(t, cfg, logger, local)
	catalog := startDeployFlowCatalog(t, raft)
	client, localRuntime := startDeployFlowAPI(t, cfg, logger, local, raft, catalog)

	return &deployFlowHarness{
		t:            t,
		ctx:          ctx,
		client:       client,
		workerCh:     workerCh,
		stopCh:       stopCh,
		localRuntime: localRuntime,
	}
}

func deployFlowConfig(t *testing.T, workerURL string) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.Cluster.Nodes = map[string]string{deployFlowRemoteNode: workerURL}
	cfg.Observability.Prometheus.Enabled = false
	cfg.Observability.OTLP.Enabled = false
	cfg.Raft.Bind = "127.0.0.1:0"
	cfg.Raft.Advertise = ""
	cfg.Raft.Data.Dir = filepath.Join(t.TempDir(), "dragonboat")
	return cfg
}

func startDeployFlowRaft(
	t *testing.T,
	cfg config.Config,
	logger *slog.Logger,
	local nodeid.Local,
) *raftsvc.Service {
	t.Helper()
	raft := raftsvc.New(cfg, logger, local)
	requireNoError(t, raft.Start(t.Context()), "start raft")
	t.Cleanup(func() {
		if err := raft.Stop(context.Background()); err != nil {
			t.Logf("stop raft: %v", err)
		}
	})
	waitRaftLeader(t.Context(), t, raft)
	return raft
}

func startDeployFlowCatalog(t *testing.T, raft *raftsvc.Service) *nodecapacity.Catalog {
	t.Helper()
	store := raftsvc.NewRaftCapacityStore(raft)
	requireNoError(t, store.Upsert(t.Context(), deployFlowCapacitySnapshot()), "upsert capacity")
	return nodecapacity.NewCatalog(store)
}

func deployFlowCapacitySnapshot() nodecapacity.Snapshot {
	return nodecapacity.Snapshot{
		NodeID:           deployFlowRemoteNode,
		UpdatedAt:        time.Now(),
		LogicalCPUCores:  8,
		CPUUsagePercent:  5,
		MemoryAvailBytes: 16 << 30,
	}
}

func startDeployFlowAPI(
	t *testing.T,
	cfg config.Config,
	logger *slog.Logger,
	local nodeid.Local,
	raft *raftsvc.Service,
	catalog *nodecapacity.Catalog,
) (*apiclient.Client, *e2eRuntimeProvider) {
	t.Helper()
	obs, err := observability.New(cfg, nil, logger)
	requireNoError(t, err, "new observability")
	metricsSvc := metrics.New(obs)
	localRuntime := &e2eRuntimeProvider{}
	runtimeManager := orchruntime.NewManager(logger, localRuntime)
	registrySvc := registry.NewService(logger)
	taskSvc := newDeployFlowTaskService(logger, metricsSvc, runtimeManager, registrySvc, cfg, local, raft, catalog)
	taskSvc.StartDeployReconcile(t.Context())

	fiberApp, rt := newE2EServerRuntime(logger)
	api.Register(rt, cfg, registrySvc, taskSvc, newE2ELoader(t), runtimeManager, nil, raft)
	client, err := apiclient.New(startTestFiberServer(t, fiberApp), "")
	requireNoError(t, err, "new api client")
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("close client: %v", err)
		}
	})
	return client, localRuntime
}

func newDeployFlowTaskService(
	logger *slog.Logger,
	metricsSvc *metrics.Service,
	runtimeManager *orchruntime.Manager,
	registrySvc *registry.Service,
	cfg config.Config,
	local nodeid.Local,
	raft *raftsvc.Service,
	catalog *nodecapacity.Catalog,
) *task.Service {
	return task.NewService(logger, metricsSvc, runtimeManager, registrySvc, cfg, task.Bundle{
		LocalNode:  local,
		Catalog:    catalog,
		Placement:  placement.NewEngine(),
		Raft:       raft,
		Dispatcher: task.NewHTTPWorkerDispatcher(cfg),
	})
}

func waitRaftLeader(ctx context.Context, t *testing.T, raft *raftsvc.Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err := raft.Status(ctx)
		if err == nil && status.Ready && status.IsLeader {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("raft did not become leader: status=%#v error=%v", status, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newE2ELoader(t *testing.T) *loader.Loader {
	t.Helper()
	compiler, err := deployorch.NewCompiler()
	requireNoError(t, err, "new orch compiler")
	orchLoader, err := deployorch.NewOrch(compiler)
	requireNoError(t, err, "new orch loader")
	loaderSvc, err := loader.NewLoader(orchLoader)
	requireNoError(t, err, "new manifest loader")
	return loaderSvc
}

func newE2EServerRuntime(logger *slog.Logger) (*fiber.App, httpx.ServerRuntime) {
	fiberApp := fiber.New()
	fiberAdapter := adapterfiber.New(fiberApp, adapter.HumaOptions{
		Title:       "orch API test",
		Version:     "test",
		Description: "orch API test",
	})
	rt := httpx.New(
		httpx.WithAdapter(fiberAdapter),
		httpx.WithLogger(logger),
		httpx.WithValidation(),
		httpx.WithBasePath("/api"),
	)
	return fiberApp, rt
}

func startTestFiberServer(t *testing.T, app *fiber.App) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	requireNoError(t, err, "listen")
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Logf("shutdown fiber app: %v", err)
		}
		select {
		case err := <-errCh:
			if err != nil {
				t.Logf("fiber listener stopped: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Log("timed out waiting for fiber listener shutdown")
		}
	})
	return "http://" + ln.Addr().String()
}

func requireNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}
