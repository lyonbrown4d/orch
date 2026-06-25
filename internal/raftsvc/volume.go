package raftsvc

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) ApplyVolumeBinding(ctx context.Context, binding volumemeta.Binding) error {
	if s == nil {
		return oopsx.B("raft").Errorf("nil service")
	}
	binding.Key = strings.TrimSpace(binding.Key)
	binding.Metadata.Name = strings.TrimSpace(binding.Metadata.Name)
	binding.Metadata.Namespace = strings.TrimSpace(binding.Metadata.Namespace)
	binding.Volume = strings.TrimSpace(binding.Volume)
	binding.Workload = strings.TrimSpace(binding.Workload)
	binding.Target = strings.TrimSpace(binding.Target)
	binding.Node = strings.TrimSpace(binding.Node)
	binding.Source = strings.TrimSpace(binding.Source)
	binding.Status = strings.TrimSpace(binding.Status)
	if binding.Metadata.Name == "" {
		return oopsx.B("raft").Errorf("volume binding metadata.name is required")
	}
	if binding.Workload == "" {
		return oopsx.B("raft").Errorf("volume binding workload is required")
	}
	if binding.Volume == "" {
		return oopsx.B("raft").Errorf("volume binding volume is required")
	}
	if binding.Key == "" {
		binding.Key = volumemeta.BindingKey(binding.Metadata, binding.Workload, binding.Volume)
	}
	if binding.Key == "" {
		return oopsx.B("raft").Errorf("volume binding key is required")
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = time.Now().UTC()
	}

	b, err := json.Marshal(struct {
		Type    string             `json:"type"`
		Binding volumemeta.Binding `json:"binding"`
	}{
		Type:    cmdUpsertVolumeBinding,
		Binding: binding,
	})
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "marshal volume binding command")
	}
	return s.applyCommand(ctx, b, 5*time.Second, "not leader: send volume binding to the raft leader node")
}

func (s *Service) ListVolumeBindings() *list.List[volumemeta.Binding] {
	if s == nil || s.fsm == nil {
		return list.NewList[volumemeta.Binding]()
	}
	out := s.fsm.listVolumeBindings()
	out.Sort(func(a, b volumemeta.Binding) int {
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

func (s *Service) GetVolumeBinding(key string) (volumemeta.Binding, bool) {
	if s == nil || s.fsm == nil {
		return volumemeta.Binding{}, false
	}
	return s.fsm.getVolumeBinding(key)
}
