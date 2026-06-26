package v1alpha1

import (
	"strings"
	"time"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (w *Workload) validateLifecycle() error {
	if w.Lifecycle == nil {
		return nil
	}
	if err := validateRestartPolicy(w.Lifecycle.RestartPolicy); err != nil {
		return err
	}
	if w.Lifecycle.MaxRestarts < 0 {
		return oopsx.B("deploy").Errorf("lifecycle.maxRestarts must be >= 0")
	}
	if strings.TrimSpace(w.Lifecycle.RestartDelay) == "" {
		return nil
	}
	if _, err := time.ParseDuration(w.Lifecycle.RestartDelay); err != nil {
		return oopsx.B("deploy").Wrapf(err, "lifecycle.restartDelay is invalid")
	}
	return nil
}

func validateRestartPolicy(policy RestartPolicy) error {
	switch RestartPolicy(strings.TrimSpace(string(policy))) {
	case "", RestartPolicyAlways, RestartPolicyOnFailure, RestartPolicyNever:
		return nil
	default:
		return oopsx.B("deploy").Errorf("lifecycle.restartPolicy must be one of always, on-failure, never")
	}
}
