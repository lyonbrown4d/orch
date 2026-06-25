package services

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/dix"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/lifecycleplan"
	"github.com/lyonbrown4d/orch/internal/nodecapacity"
	"github.com/lyonbrown4d/orch/internal/placement"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func Module() dix.Module {
	return dix.NewModule(
		"services",
		dix.Providers(
			dix.Provider0(placement.NewEngine),
			dix.Provider1(func(rs *raftsvc.Service) *nodecapacity.Catalog {
				return nodecapacity.NewCatalog(raftsvc.NewRaftCapacityStore(rs))
			}),
			dix.Provider1(func(cfg config.Config) task.WorkerDispatcher {
				return task.NewHTTPWorkerDispatcher(cfg)
			}),
			dix.Provider6(task.NewBundle),
			dix.Provider1(registry.NewService),
			dix.Provider6(task.NewService, dix.Eager()),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, tasks *task.Service) error {
				tasks.StartDeployReconcile(context.WithoutCancel(ctx))
				return nil
			}, dix.LifecycleName(lifecycleplan.HookTaskReconcile), dix.LifecyclePriority(lifecycleplan.PriorityWorkload), dix.LifecycleTimeout(lifecycleplan.TimeoutReady)),
			dix.OnStop2(func(ctx context.Context, logger *slog.Logger, tasks *task.Service) error {
				logger.Info("lifecycle", "phase", "stopping", "component", "task-reconcile")
				if err := tasks.StopDeployReconcile(ctx); err != nil {
					logger.Warn("lifecycle", "phase", "stop_failed", "component", "task-reconcile", "error", err)
					return oopsx.B("services").Wrapf(err, "stop task reconcile")
				}
				logger.Info("lifecycle", "phase", "stopped", "component", "task-reconcile")
				return nil
			}, dix.LifecycleName(lifecycleplan.HookTaskReconcile), dix.LifecyclePriority(lifecycleplan.PriorityWorkload), dix.LifecycleTimeout(lifecycleplan.TimeoutStop)),
		),
	)
}
