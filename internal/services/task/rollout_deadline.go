package task

import (
	"context"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func withRolloutProgressDeadline(ctx context.Context, workload deployv1.Workload) (context.Context, context.CancelFunc) {
	deadline, err := workload.Rollout.ProgressDeadlineDuration()
	if err != nil || deadline <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, deadline)
}
