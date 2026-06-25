package task

import (
	"context"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
)

func (s *Service) applyWorkloadVolumeBindings(ctx context.Context, app *deployv1.App, workload deployv1.Workload, nodeID, status, generation, errMsg string) {
	if s == nil || s.raft == nil || app == nil || len(workload.Mounts) == 0 {
		return
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = volumemeta.BindingStatusBound
	}
	specs := volumeSpecsByName(app)
	workload.MountList().Range(func(_ int, mount deployv1.Mount) bool {
		volumeName := strings.TrimSpace(mount.Volume.Name)
		if volumeName == "" {
			return true
		}
		volume := specs[volumeName]
		binding := volumemeta.Binding{
			Key:        volumemeta.BindingKey(app.Metadata, workload.Name, volumeName),
			Metadata:   app.Metadata,
			Volume:     volumeName,
			Workload:   workload.Name,
			Target:     strings.TrimSpace(mount.Target),
			Node:       strings.TrimSpace(nodeID),
			Runtime:    workload.Runtime,
			Source:     runconfig.VolumeName(app.Metadata, volumeName),
			Persistent: volume.Persistent,
			SizeBytes:  volume.SizeBytes,
			Status:     status,
			Generation: strings.TrimSpace(generation),
			Error:      strings.TrimSpace(errMsg),
			UpdatedAt:  time.Now().UTC(),
		}
		if err := s.raft.ApplyVolumeBinding(ctx, binding); err != nil {
			s.logger.Warn("volume binding apply",
				"error", err,
				"app", app.Metadata.Name,
				"volume", volumeName,
				"workload", workload.Name,
				"node", nodeID,
				"status", status,
			)
		}
		return true
	})
}

func volumeSpecsByName(app *deployv1.App) map[string]deployv1.Volume {
	if app == nil || len(app.Volumes) == 0 {
		return map[string]deployv1.Volume{}
	}
	out := make(map[string]deployv1.Volume, len(app.Volumes))
	app.VolumeList().Range(func(_ int, volume deployv1.Volume) bool {
		name := strings.TrimSpace(volume.Name)
		if name != "" {
			volume.Name = name
			out[name] = volume
		}
		return true
	})
	return out
}
