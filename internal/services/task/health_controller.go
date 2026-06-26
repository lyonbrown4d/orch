package task

import (
	"context"
	"fmt"
	"strings"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

type workloadHealthMonitor struct {
	id     uint64
	cancel context.CancelFunc
}

type workloadHealthTarget struct {
	app        deployv1.App
	workload   deployv1.Workload
	nodeID     string
	generation string
	address    string
	key        string
}

func (s *Service) startLocalWorkloadHealthMonitor(ctx context.Context, app *deployv1.App, workload deployv1.Workload, nodeID, generation, address string) {
	if !s.shouldStartLocalHealthMonitor(workload, nodeID) {
		return
	}
	target := s.newWorkloadHealthTarget(app, workload, nodeID, generation, address)
	if target.key == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	runID := s.registerWorkloadHealthMonitor(target.key, cancel)
	go s.runWorkloadHealthMonitor(ctx, runID, target)
}

func (s *Service) shouldStartLocalHealthMonitor(workload deployv1.Workload, nodeID string) bool {
	return s != nil && s.local.String() == strings.TrimSpace(nodeID) && workloadHasBackgroundHealth(workload)
}

func workloadHasBackgroundHealth(workload deployv1.Workload) bool {
	return workloadStartupProbe(workload) != nil || workloadLivenessProbe(workload) != nil
}

func workloadStartupProbe(workload deployv1.Workload) *deployv1.Probe {
	if workload.Health == nil {
		return nil
	}
	return workload.Health.Startup
}

func workloadLivenessProbe(workload deployv1.Workload) *deployv1.Probe {
	if workload.Health == nil {
		return nil
	}
	return workload.Health.Liveness
}

func (s *Service) newWorkloadHealthTarget(app *deployv1.App, workload deployv1.Workload, nodeID, generation, address string) workloadHealthTarget {
	current := deployv1.App{Metadata: deployv1.Metadata{}, Workloads: []deployv1.Workload{workload}}
	if app != nil {
		current = *app
	}
	if strings.TrimSpace(generation) == "" {
		generation = AppGeneration(current)
	}
	return workloadHealthTarget{
		app:        current,
		workload:   workload,
		nodeID:     strings.TrimSpace(nodeID),
		generation: strings.TrimSpace(generation),
		address:    strings.TrimSpace(address),
		key:        workloadmeta.AssignmentKey(current.Metadata, workload.Name),
	}
}

func (s *Service) registerWorkloadHealthMonitor(key string, cancel context.CancelFunc) uint64 {
	var oldCancel context.CancelFunc
	s.healthMu.Lock()
	if s.healthMonitors == nil {
		s.healthMonitors = make(map[string]workloadHealthMonitor)
	}
	if old, ok := s.healthMonitors[key]; ok {
		oldCancel = old.cancel
	}
	s.healthRun++
	runID := s.healthRun
	s.healthMonitors[key] = workloadHealthMonitor{id: runID, cancel: cancel}
	s.healthMu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
	return runID
}

func (s *Service) stopWorkloadHealthMonitor(meta deployv1.Metadata, workloadName string) {
	if s == nil {
		return
	}
	key := workloadmeta.AssignmentKey(meta, workloadName)
	if key == "" {
		return
	}
	var cancel context.CancelFunc
	s.healthMu.Lock()
	if current, ok := s.healthMonitors[key]; ok {
		cancel = current.cancel
		delete(s.healthMonitors, key)
	}
	s.healthMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) StopWorkloadHealthMonitors() {
	if s == nil {
		return
	}
	var cancels []context.CancelFunc
	s.healthMu.Lock()
	for key, current := range s.healthMonitors {
		if current.cancel != nil {
			cancels = append(cancels, current.cancel)
		}
		delete(s.healthMonitors, key)
	}
	s.healthMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
func (s *Service) finishWorkloadHealthMonitor(key string, runID uint64) {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	current, ok := s.healthMonitors[key]
	if ok && current.id == runID {
		delete(s.healthMonitors, key)
	}
}

func (s *Service) runWorkloadHealthMonitor(ctx context.Context, runID uint64, target workloadHealthTarget) {
	defer s.finishWorkloadHealthMonitor(target.key, runID)
	if err := s.runStartupHealth(ctx, target); err != nil {
		if ctx.Err() == nil {
			s.failLocalWorkloadHealth(ctx, target, "startup", err)
		}
		return
	}
	if err := s.runLivenessHealth(ctx, target); err != nil && ctx.Err() == nil {
		s.failLocalWorkloadHealth(ctx, target, "liveness", err)
	}
}

func (s *Service) runStartupHealth(ctx context.Context, target workloadHealthTarget) error {
	probe := workloadStartupProbe(target.workload)
	if probe == nil {
		return nil
	}
	check, err := s.buildReadinessCheck(target.app.Metadata, target.workload, *probe, target.address)
	if err != nil {
		return err
	}
	if err := sleepContext(ctx, check.startPeriod); err != nil {
		return err
	}
	return s.waitHealthProbeAttempts(ctx, target.workload, "startup", check)
}

func (s *Service) runLivenessHealth(ctx context.Context, target workloadHealthTarget) error {
	probe := workloadLivenessProbe(target.workload)
	if probe == nil {
		return nil
	}
	check, err := s.buildReadinessCheck(target.app.Metadata, target.workload, *probe, target.address)
	if err != nil {
		return err
	}
	if err := sleepContext(ctx, check.startPeriod); err != nil {
		return err
	}
	for {
		if err := s.waitHealthProbeAttempts(ctx, target.workload, "liveness", check); err != nil {
			return err
		}
		if err := sleepContext(ctx, check.interval); err != nil {
			return err
		}
	}
}

func (s *Service) waitHealthProbeAttempts(ctx context.Context, workload deployv1.Workload, phase string, check readinessCheck) error {
	var lastErr error
	for attempt := 1; attempt <= check.attempts; attempt++ {
		lastErr = runReadinessCheck(ctx, check)
		if lastErr == nil {
			s.logger.Debug("workload health probe succeeded", "workload", workload.Name, "phase", phase, "kind", check.kind, "attempt", attempt)
			return nil
		}
		if err := waitBeforeNextReadinessAttempt(ctx, check, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("workload %s %s probe failed after %d attempt(s): %w", workload.Name, phase, check.attempts, lastErr)
}

func (s *Service) failLocalWorkloadHealth(ctx context.Context, target workloadHealthTarget, phase string, err error) {
	message := fmt.Sprintf("%s health failed: %v", phase, err)
	s.logger.Warn("workload health failed", "workload", target.workload.Name, "phase", phase, "error", err)
	if s.runtime != nil {
		if stopErr := s.runtime.Stop(ctx, target.workload.Runtime, target.app.Metadata, target.workload.Name); stopErr != nil {
			message = message + "; stop failed: " + stopErr.Error()
			s.logger.Warn("stop workload after health failure", "workload", target.workload.Name, "error", stopErr)
		}
	}
	if s.registry != nil {
		s.registry.Delete(target.workload.Name)
	}
	if !s.healthAssignmentStillCurrent(target) {
		return
	}
	s.applyWorkloadAssignment(ctx, target.app.Metadata, target.workload, target.nodeID, workloadmeta.AssignmentStatusFailed, target.generation, message)
	s.applyWorkloadVolumeBindings(ctx, &target.app, target.workload, target.nodeID, volumemeta.BindingStatusFailed, target.generation, message)
}

func (s *Service) healthAssignmentStillCurrent(target workloadHealthTarget) bool {
	if s == nil || s.raft == nil {
		return false
	}
	assignment, ok := s.raft.GetWorkloadAssignment(target.key)
	if !ok {
		return true
	}
	if strings.TrimSpace(assignment.Node) != target.nodeID {
		return false
	}
	if target.generation != "" && strings.TrimSpace(assignment.Generation) != target.generation {
		return false
	}
	return strings.TrimSpace(assignment.Status) == workloadmeta.AssignmentStatusRunning
}

func (s *Service) desiredHealthMonitorApp(meta deployv1.Metadata, workload deployv1.Workload) deployv1.App {
	if s != nil && s.raft != nil {
		if app, ok := s.raft.GetDesiredDeployApp(meta); ok {
			return app
		}
	}
	return deployv1.App{Metadata: meta, Workloads: []deployv1.Workload{workload}}
}
