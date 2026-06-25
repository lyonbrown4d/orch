package task

import (
	"context"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const deployReconcileInterval = 2 * time.Second

// StartDeployReconcile runs a background loop that executes [Service.deployAppWorkloads] whenever the Raft FSM
// updates desired deploy documents (and once immediately for startup catch-up).
func (s *Service) StartDeployReconcile(ctx context.Context) {
	if !s.canStartDeployReconcile() {
		return
	}
	ch := s.raft.DeployReconcileSignals()
	if ch == nil {
		return
	}
	loopCtx, cancel, runID, ok := s.beginDeployReconcile(ctx)
	if !ok {
		return
	}
	go s.runDeployReconcile(loopCtx, cancel, ch, runID)
}

func (s *Service) canStartDeployReconcile() bool {
	return s != nil && s.raft != nil
}

func (s *Service) beginDeployReconcile(ctx context.Context) (context.Context, context.CancelFunc, uint64, bool) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	if s.reconcileCancel != nil {
		return nil, nil, 0, false
	}
	ctx, cancel := context.WithCancel(ctx)
	s.reconcileRun++
	runID := s.reconcileRun
	s.reconcileCancel = cancel
	s.reconcileWG.Add(1)
	return ctx, cancel, runID, true
}

func (s *Service) runDeployReconcile(ctx context.Context, cancel context.CancelFunc, ch <-chan struct{}, runID uint64) {
	defer cancel()
	defer s.finishDeployReconcile(runID)
	s.logger.Info("deploy reconcile started")
	if !s.waitDeployReconcileLeader(ctx) {
		return
	}
	s.runDeployReconcileLoop(ctx, ch)
}

func (s *Service) waitDeployReconcileLeader(ctx context.Context) bool {
	if err := s.raft.WaitLocalLeader(ctx); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("deploy reconcile waiting for raft leader stopped", "error", err)
		}
		return false
	}
	s.logger.Info("deploy reconcile acquired raft leadership")
	return true
}

func (s *Service) runDeployReconcileLoop(ctx context.Context, ch <-chan struct{}) {
	if !s.reconcilePendingDeploys(ctx, ch) {
		return
	}
	ticker := time.NewTicker(deployReconcileInterval)
	defer ticker.Stop()
	for s.waitDeployReconcileTrigger(ctx, ch, ticker.C) {
		if !s.reconcilePendingDeploys(ctx, ch) {
			return
		}
	}
}

func (s *Service) waitDeployReconcileTrigger(ctx context.Context, ch <-chan struct{}, tick <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-ch:
		return true
	case <-tick:
		return true
	}
}

func (s *Service) finishDeployReconcile(runID uint64) {
	defer s.reconcileWG.Done()
	s.reconcileMu.Lock()
	if s.reconcileRun == runID {
		s.reconcileCancel = nil
	}
	s.reconcileMu.Unlock()
}

func (s *Service) reconcilePendingDeploys(ctx context.Context, ch <-chan struct{}) bool {
	s.reconcileAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ch:
			s.reconcileAll(ctx)
		default:
			return true
		}
	}
}

func (s *Service) StopDeployReconcile(ctx context.Context) error {
	if s == nil {
		return nil
	}
	cancel := s.takeDeployReconcileCancel()
	if cancel != nil {
		cancel()
	}
	return s.waitDeployReconcileStopped(ctx)
}

func (s *Service) takeDeployReconcileCancel() context.CancelFunc {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	cancel := s.reconcileCancel
	s.reconcileCancel = nil
	return cancel
}

func (s *Service) waitDeployReconcileStopped(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.reconcileWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return oopsx.B("task").Wrapf(ctx.Err(), "stop deploy reconcile")
	}
}

func (s *Service) reconcileAll(ctx context.Context) {
	apps := s.raft.ListDesiredDeployApps()
	if apps.Len() == 0 {
		s.logger.Debug("deploy reconcile found no desired apps")
		return
	}
	s.logger.Debug("deploy reconcile desired apps", "apps", apps.Len())
	apps.Range(func(_ int, app deployv1.App) bool {
		current := app
		if err := s.deployAppWorkloads(ctx, &current); err != nil {
			s.logger.Warn("deploy reconcile", "error", err, "app", current.Metadata.Name)
		}
		return true
	})
}
