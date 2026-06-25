package api

import (
	"context"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/httpx"

	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
)

// VolumesEndpoint serves GET /api/v1/volumes.
type VolumesEndpoint struct {
	tasks *task.Service
}

func NewVolumesEndpoint(tasks *task.Service) *VolumesEndpoint {
	return &VolumesEndpoint{tasks: tasks}
}

func (e *VolumesEndpoint) EndpointSpec() httpx.EndpointSpec {
	return httpx.EndpointSpec{
		Prefix:      "/v1/volumes",
		Description: "Local runtime volume binding state replicated through Raft.",
		Tags:        httpx.Tags("scheduler", "storage"),
	}
}

func (e *VolumesEndpoint) Register(r httpx.Registrar) {
	httpx.MustGroupGet(r.Scope(), "", e.Handle, OpenAPIMeta([]string{"scheduler", "storage"}, "listVolumeBindings", "List local runtime volume bindings",
		"Sorted volume binding records including app metadata, volume name, workload target, assigned node, runtime source, status, and last error."))
}

// Handle returns the current local runtime volume binding view.
func (e *VolumesEndpoint) Handle(_ context.Context, _ *EmptyInput) (*ListVolumesOutput, error) {
	out := &ListVolumesOutput{}
	out.Body.Items = list.NewList[VolumeBindingItem]()
	if e != nil && e.tasks != nil {
		out.Body.Items = list.MapList(e.tasks.ListVolumeBindings(), func(_ int, binding volumemeta.Binding) VolumeBindingItem {
			return VolumeBindingItem{
				Key:        binding.Key,
				Metadata:   binding.Metadata,
				Volume:     binding.Volume,
				Workload:   binding.Workload,
				Target:     binding.Target,
				Node:       binding.Node,
				Runtime:    binding.Runtime,
				Source:     binding.Source,
				Persistent: binding.Persistent,
				SizeBytes:  binding.SizeBytes,
				Status:     binding.Status,
				Generation: binding.Generation,
				Error:      binding.Error,
				UpdatedAt:  binding.UpdatedAt,
			}
		})
	}
	return out, nil
}
