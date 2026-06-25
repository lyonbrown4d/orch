package api_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
)

func TestVolumesEndpointHandle(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := slog.New(slog.DiscardHandler)
	raft := raftsvc.New(cfg, logger, nodeid.Local{Value: "node-a"})
	meta := deployv1.Metadata{Name: "demo", Namespace: "default"}
	if err := raft.ApplyVolumeBinding(context.Background(), volumemeta.Binding{
		Metadata:   meta,
		Volume:     "data",
		Workload:   "db",
		Target:     "/data",
		Node:       "node-a",
		Runtime:    deployv1.RuntimeDocker,
		Source:     "orch_default_demo_data",
		Persistent: true,
		Status:     volumemeta.BindingStatusBound,
	}); err != nil {
		t.Fatal(err)
	}

	tasks := task.NewService(logger, nil, nil, nil, cfg, task.Bundle{Raft: raft})
	out, err := api.NewVolumesEndpoint(tasks).Handle(context.Background(), &api.EmptyInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Body.Items.Len() != 1 {
		t.Fatalf("items = %#v", out.Body.Items)
	}
	got, ok := out.Body.Items.Get(0)
	if !ok {
		t.Fatal("missing volume item")
	}
	if got.Key != volumemeta.BindingKey(meta, "db", "data") || got.Node != "node-a" || got.Status != volumemeta.BindingStatusBound || got.Source != "orch_default_demo_data" {
		t.Fatalf("volume binding = %#v", got)
	}
}
