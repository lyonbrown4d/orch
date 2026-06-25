package workerapi

import (
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
)

const PathV1WorkerDeploy = "/api/v1/worker/deploy"
const PathV1WorkerStop = "/api/v1/worker/stop"
const PathV1WorkerStatus = "/api/v1/worker/status"
const PathV1WorkerLogs = "/api/v1/worker/logs"

type DeployWorkloadBody struct {
	Metadata deployv1.Metadata `json:"metadata"`
	Workload deployv1.Workload `json:"workload"`
	Node     string            `json:"node,omitempty"`
}

type DeployWorkloadInput struct {
	Body DeployWorkloadBody `json:"body"`
}

type DeployWorkloadOutput struct {
	Body struct {
		Accepted bool   `json:"accepted"`
		Node     string `json:"node"`
		Address  string `json:"address,omitempty"`
		Status   string `json:"status"`
		Workload string `json:"workload"`
	} `json:"body"`
}

type StopWorkloadBody struct {
	Metadata deployv1.Metadata `json:"metadata"`
	Workload deployv1.Workload `json:"workload"`
	Node     string            `json:"node,omitempty"`
}

type StopWorkloadInput struct {
	Body StopWorkloadBody `json:"body"`
}

type StopWorkloadOutput struct {
	Body struct {
		Accepted bool   `json:"accepted"`
		Node     string `json:"node"`
		Status   string `json:"status"`
		Workload string `json:"workload"`
	} `json:"body"`
}

type WorkloadStatusBody struct {
	Metadata deployv1.Metadata `json:"metadata"`
	Workload deployv1.Workload `json:"workload"`
	Node     string            `json:"node,omitempty"`
}

type WorkloadStatusInput struct {
	Body WorkloadStatusBody `json:"body"`
}

type WorkloadStatusOutput struct {
	Body runtimeinfo.Status `json:"body"`
}

type WorkloadLogsBody struct {
	Metadata deployv1.Metadata `json:"metadata"`
	Workload deployv1.Workload `json:"workload"`
	Node     string            `json:"node,omitempty"`
	Tail     int               `json:"tail,omitempty"`
}

type WorkloadLogsInput struct {
	Body WorkloadLogsBody `json:"body"`
}

type WorkloadLogsOutput struct {
	Body runtimeinfo.LogResult `json:"body"`
}
