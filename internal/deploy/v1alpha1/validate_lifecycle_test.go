package v1alpha1_test

import (
	"strings"
	"testing"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func TestValidateLifecycleAllowsRestartPolicy(t *testing.T) {
	app := validApp()
	app.Workloads[0].Lifecycle = &deployv1.Lifecycle{
		RestartPolicy: deployv1.RestartPolicyOnFailure,
		MaxRestarts:   3,
		RestartDelay:  "5s",
	}

	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with lifecycle = %v", err)
	}
}

func TestValidateLifecycleRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle deployv1.Lifecycle
		want      string
	}{
		{name: "policy", lifecycle: deployv1.Lifecycle{RestartPolicy: "sometimes"}, want: "lifecycle.restartPolicy must be one of always, on-failure, never"},
		{name: "maxRestarts", lifecycle: deployv1.Lifecycle{MaxRestarts: -1}, want: "lifecycle.maxRestarts must be >= 0"},
		{name: "restartDelay", lifecycle: deployv1.Lifecycle{RestartDelay: "soon"}, want: "lifecycle.restartDelay is invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := validApp()
			app.Workloads[0].Lifecycle = &tt.lifecycle

			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}
