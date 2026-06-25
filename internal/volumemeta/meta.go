// Package volumemeta defines replicated local volume lifecycle records.
package volumemeta

import (
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

const (
	BindingStatusBound    = "bound"
	BindingStatusReleased = "released"
	BindingStatusFailed   = "failed"
)

type Binding struct {
	Key        string               `json:"key"`
	Metadata   deployv1.Metadata    `json:"metadata"`
	Volume     string               `json:"volume"`
	Workload   string               `json:"workload,omitempty"`
	Target     string               `json:"target,omitempty"`
	Node       string               `json:"node"`
	Runtime    deployv1.RuntimeKind `json:"runtime"`
	Source     string               `json:"source,omitempty"`
	Persistent bool                 `json:"persistent"`
	SizeBytes  int64                `json:"sizeBytes,omitempty"`
	Status     string               `json:"status"`
	Generation string               `json:"generation,omitempty"`
	Error      string               `json:"error,omitempty"`
	UpdatedAt  time.Time            `json:"updatedAt"`
}

func BindingKey(meta deployv1.Metadata, workloadName, volumeName string) string {
	ns := workloadmeta.NamespaceOrDefault(meta.Namespace)
	app := strings.TrimSpace(meta.Name)
	workload := strings.TrimSpace(workloadName)
	volume := strings.TrimSpace(volumeName)
	if app == "" || workload == "" || volume == "" {
		return ""
	}
	return ns + "/" + app + "/" + workload + "/" + volume
}
