package task

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/metrics"
	"github.com/lyonbrown4d/orch/internal/nodecapacity"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/placement"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/services/registry"
)

type Service struct {
	logger     *slog.Logger
	cfg        config.Config
	metrics    *metrics.Service
	runtime    *runtime.Manager
	registry   *registry.Service
	catalog    *nodecapacity.Catalog
	placement  *placement.Engine
	local      nodeid.Local
	raft       *raftsvc.Service
	dispatcher WorkerDispatcher
	dns        *dnssvc.Service

	reconcileMu     sync.Mutex
	reconcileCancel context.CancelFunc
	reconcileRun    uint64
	reconcileWG     sync.WaitGroup

	healthMu       sync.Mutex
	healthRun      uint64
	healthMonitors map[string]workloadHealthMonitor

	recoveryMu     sync.Mutex
	recoveryCancel context.CancelFunc
	recoveryRun    uint64
	recoveryWG     sync.WaitGroup

	restartMu       sync.Mutex
	restartAttempts map[string]int
}

func NewService(logger *slog.Logger, metricService *metrics.Service, runtimeManager *runtime.Manager, registryService *registry.Service, cfg config.Config, bundle Bundle) *Service {
	return &Service{
		logger:     logger,
		cfg:        cfg,
		metrics:    metricService,
		runtime:    runtimeManager,
		registry:   registryService,
		catalog:    bundle.Catalog,
		placement:  bundle.Placement,
		local:      bundle.LocalNode,
		raft:       bundle.Raft,
		dispatcher: bundle.Dispatcher,
		dns:        bundle.DNS,
	}
}
