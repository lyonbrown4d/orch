package task

import (
	"context"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

const autoRollbackApplyTimeout = 30 * time.Second

func (s *Service) maybeAutoRollbackDeployFailure(ctx context.Context, app *deployv1.App, workload deployv1.Workload, generation string, cause error) {
	revision, ok := s.autoRollbackRevision(app, workload, generation)
	if !ok {
		return
	}

	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), autoRollbackApplyTimeout)
	defer cancel()
	if err := s.preflightDeploy(rollbackCtx, &revision.App); err != nil {
		s.logger.Warn("deploy auto rollback preflight failed",
			"app", app.Metadata.Name,
			"namespace", workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
			"workload", workload.Name,
			"from_generation", strings.TrimSpace(generation),
			"to_generation", revision.Generation,
			"error", err,
		)
		return
	}

	s.suppressAutoRollback(app.Metadata, revision.Generation)
	if err := s.raft.ApplyDeployApp(rollbackCtx, revision.App); err != nil {
		s.logger.Warn("deploy auto rollback apply failed",
			"app", app.Metadata.Name,
			"namespace", workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
			"workload", workload.Name,
			"from_generation", strings.TrimSpace(generation),
			"to_generation", revision.Generation,
			"error", err,
		)
		return
	}

	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	s.logger.Warn("deploy auto rollback submitted",
		"app", app.Metadata.Name,
		"namespace", workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
		"workload", workload.Name,
		"from_generation", strings.TrimSpace(generation),
		"to_generation", revision.Generation,
		"reason", reason,
	)
}

func (s *Service) autoRollbackRevision(app *deployv1.App, workload deployv1.Workload, generation string) (raftsvc.DeployAppRevision, bool) {
	generation = strings.TrimSpace(generation)
	if !s.autoRollbackConfigured(app, workload, generation) {
		return raftsvc.DeployAppRevision{}, false
	}
	if !s.autoRollbackDesiredGenerationCurrent(app.Metadata, generation) {
		return raftsvc.DeployAppRevision{}, false
	}
	revision, ok := s.previousAutoRollbackRevision(app.Metadata, generation)
	if !ok || !s.markAutoRollbackAttempt(app.Metadata, generation) {
		return raftsvc.DeployAppRevision{}, false
	}
	return revision, true
}

func (s *Service) autoRollbackConfigured(app *deployv1.App, workload deployv1.Workload, generation string) bool {
	switch {
	case s == nil:
		return false
	case s.raft == nil:
		return false
	case app == nil:
		return false
	case generation == "":
		return false
	case !workload.Rollout.RollbackOnFailureEnabled():
		return false
	case s.autoRollbackIsSuppressed(app.Metadata, generation):
		return false
	default:
		return true
	}
}

func (s *Service) autoRollbackDesiredGenerationCurrent(meta deployv1.Metadata, generation string) bool {
	current, ok := s.raft.GetDesiredDeployApp(meta)
	return ok && strings.TrimSpace(AppGeneration(current)) == generation
}

func (s *Service) previousAutoRollbackRevision(meta deployv1.Metadata, generation string) (raftsvc.DeployAppRevision, bool) {
	revision, ok := s.raft.GetPreviousDeployApp(meta)
	if !ok {
		return raftsvc.DeployAppRevision{}, false
	}
	target := strings.TrimSpace(revision.Generation)
	return revision, target != "" && target != generation
}

func (s *Service) markAutoRollbackAttempt(meta deployv1.Metadata, generation string) bool {
	key := autoRollbackGuardKey(meta, generation)
	if key == "" {
		return false
	}
	s.autoRollbackMu.Lock()
	defer s.autoRollbackMu.Unlock()
	if s.autoRollbackAttempts == nil {
		s.autoRollbackAttempts = map[string]struct{}{}
	}
	if _, ok := s.autoRollbackAttempts[key]; ok {
		return false
	}
	s.autoRollbackAttempts[key] = struct{}{}
	return true
}

func (s *Service) suppressAutoRollback(meta deployv1.Metadata, generation string) {
	key := autoRollbackGuardKey(meta, generation)
	if key == "" {
		return
	}
	s.autoRollbackMu.Lock()
	defer s.autoRollbackMu.Unlock()
	if s.autoRollbackSuppressed == nil {
		s.autoRollbackSuppressed = map[string]struct{}{}
	}
	s.autoRollbackSuppressed[key] = struct{}{}
}

func (s *Service) autoRollbackIsSuppressed(meta deployv1.Metadata, generation string) bool {
	key := autoRollbackGuardKey(meta, generation)
	if key == "" {
		return false
	}
	s.autoRollbackMu.Lock()
	defer s.autoRollbackMu.Unlock()
	_, ok := s.autoRollbackSuppressed[key]
	return ok
}

func autoRollbackGuardKey(meta deployv1.Metadata, generation string) string {
	name := strings.TrimSpace(meta.Name)
	generation = strings.TrimSpace(generation)
	if name == "" || generation == "" {
		return ""
	}
	return workloadmeta.NamespaceOrDefault(meta.Namespace) + "/" + name + "@" + generation
}
