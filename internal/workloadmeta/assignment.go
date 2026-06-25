package workloadmeta

import (
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

type Assignment struct {
	Key        string               `json:"key"`
	Metadata   deployv1.Metadata    `json:"metadata"`
	Workload   string               `json:"workload"`
	Node       string               `json:"node"`
	Address    string               `json:"address,omitempty"`
	Runtime    deployv1.RuntimeKind `json:"runtime"`
	Artifact   string               `json:"artifact"`
	Status     string               `json:"status"`
	Generation string               `json:"generation,omitempty"`
	Error      string               `json:"error,omitempty"`
	UpdatedAt  time.Time            `json:"updatedAt"`
}

const (
	AssignmentStatusAssigned = "assigned"
	AssignmentStatusFailed   = "failed"
	AssignmentStatusRunning  = "running"
	AssignmentStatusStopped  = "stopped"
)

func AssignmentKey(meta deployv1.Metadata, workloadName string) string {
	ns := NamespaceOrDefault(meta.Namespace)
	app := strings.TrimSpace(meta.Name)
	workload := strings.TrimSpace(workloadName)
	if app == "" || workload == "" {
		return ""
	}
	return ns + "/" + app + "/" + workload
}
