package orch

import (
	"errors"
	"strings"

	"github.com/arcgolabs/plano/compiler"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func fillWorkloadRollout(workload *v1.Workload, f *compiler.HIRForm) error {
	blocks := childFormsByKind(f, "rollout")
	if len(blocks) > 1 {
		return errors.New("at most one rollout block")
	}
	rollout := lowerRolloutFromFields(f)
	if len(blocks) == 1 {
		rollout = mergeRollout(rollout, lowerRolloutFromFields(&blocks[0]))
	}
	if rolloutEmpty(rollout) {
		return nil
	}
	workload.Rollout = rollout
	return nil
}

func lowerRolloutFromFields(f *compiler.HIRForm) *v1.Rollout {
	rollout := &v1.Rollout{}
	if strategy, ok := stringField(f, "strategy"); ok {
		rollout.Strategy = strings.TrimSpace(strategy)
	}
	if maxUnavailable, ok := intField(f, "max_unavailable"); ok {
		rollout.MaxUnavailable = maxUnavailable
	}
	if maxSurge, ok := intField(f, "max_surge"); ok {
		rollout.MaxSurge = maxSurge
	}
	if rollbackOnFailure, ok := boolField(f, "rollback_on_failure"); ok {
		rollout.RollbackOnFailure = rollbackOnFailure
	}
	if progressDeadline, ok := stringField(f, "progress_deadline"); ok {
		rollout.ProgressDeadline = strings.TrimSpace(progressDeadline)
	}
	return rollout
}

func mergeRollout(base, override *v1.Rollout) *v1.Rollout {
	if base == nil {
		base = &v1.Rollout{}
	}
	if override == nil {
		return base
	}
	if override.Strategy != "" {
		base.Strategy = override.Strategy
	}
	if override.MaxUnavailable != 0 {
		base.MaxUnavailable = override.MaxUnavailable
	}
	if override.MaxSurge != 0 {
		base.MaxSurge = override.MaxSurge
	}
	if override.RollbackOnFailure {
		base.RollbackOnFailure = true
	}
	if override.ProgressDeadline != "" {
		base.ProgressDeadline = override.ProgressDeadline
	}
	return base
}

func rolloutEmpty(rollout *v1.Rollout) bool {
	if rollout == nil {
		return true
	}
	return rollout.Strategy == "" && rollout.MaxUnavailable == 0 && rollout.MaxSurge == 0 && !rollout.RollbackOnFailure && rollout.ProgressDeadline == ""
}
