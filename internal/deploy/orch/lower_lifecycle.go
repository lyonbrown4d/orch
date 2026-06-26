package orch

import (
	"errors"
	"strings"

	"github.com/arcgolabs/plano/compiler"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func fillWorkloadLifecycle(workload *v1.Workload, f *compiler.HIRForm) error {
	blocks := childFormsByKind(f, "lifecycle")
	if len(blocks) > 1 {
		return errors.New("at most one lifecycle block")
	}
	lifecycle := lowerLifecycleFromFields(f)
	if len(blocks) == 1 {
		lifecycle = mergeLifecycle(lifecycle, lowerLifecycleFromFields(&blocks[0]))
	}
	if lifecycleEmpty(lifecycle) {
		return nil
	}
	workload.Lifecycle = lifecycle
	return nil
}

func lowerLifecycleFromFields(f *compiler.HIRForm) *v1.Lifecycle {
	lifecycle := &v1.Lifecycle{}
	if policy, ok := stringField(f, "restart_policy"); ok {
		lifecycle.RestartPolicy = v1.RestartPolicy(strings.TrimSpace(policy))
	}
	if maxRestarts, ok := intField(f, "max_restarts"); ok {
		lifecycle.MaxRestarts = maxRestarts
	}
	if restartDelay, ok := stringField(f, "restart_delay"); ok {
		lifecycle.RestartDelay = strings.TrimSpace(restartDelay)
	}
	return lifecycle
}

func mergeLifecycle(base, override *v1.Lifecycle) *v1.Lifecycle {
	if base == nil {
		base = &v1.Lifecycle{}
	}
	if override == nil {
		return base
	}
	if override.RestartPolicy != "" {
		base.RestartPolicy = override.RestartPolicy
	}
	if override.MaxRestarts != 0 {
		base.MaxRestarts = override.MaxRestarts
	}
	if override.RestartDelay != "" {
		base.RestartDelay = override.RestartDelay
	}
	return base
}

func lifecycleEmpty(lifecycle *v1.Lifecycle) bool {
	if lifecycle == nil {
		return true
	}
	return lifecycle.RestartPolicy == "" && lifecycle.MaxRestarts == 0 && lifecycle.RestartDelay == ""
}
