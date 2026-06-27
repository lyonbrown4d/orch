package workloadmeta_test

import (
	"testing"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestOrchContainerNameIncludesApp(t *testing.T) {
	meta := deployv1.Metadata{Name: "Host.Services", Namespace: "Demo_NS"}

	got := workloadmeta.OrchContainerName(meta, "MySQL")
	want := "orch-demo_ns-host.services-mysql"
	if got != want {
		t.Fatalf("OrchContainerName() = %q, want %q", got, want)
	}
}

func TestOrchContainerNameDefaultsEmptyParts(t *testing.T) {
	got := workloadmeta.OrchContainerName(deployv1.Metadata{}, " ")
	want := "orch-default-x-x"
	if got != want {
		t.Fatalf("OrchContainerName() = %q, want %q", got, want)
	}
}
