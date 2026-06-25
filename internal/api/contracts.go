package api

import (
	"time"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dixdiag"
	"github.com/lyonbrown4d/orch/internal/hostinfo"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
)

// EmptyInput is the request shape for handlers with no parameters or body.
type EmptyInput struct{}

// HealthOutput is the response body envelope for GET PathHealth.
type HealthOutput struct {
	Body struct {
		Healthy   bool                       `json:"healthy"`
		Status    string                     `json:"status"`
		Timestamp string                     `json:"timestamp"`
		Checks    *list.List[ReadyCheckItem] `json:"checks,omitempty"`
	} `json:"body"`
}

type ReadyCheckItem struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// ReadyOutput is the response body envelope for GET PathReady.
type ReadyOutput struct {
	Body struct {
		Ready     bool                       `json:"ready"`
		Status    string                     `json:"status"`
		Timestamp string                     `json:"timestamp"`
		Checks    *list.List[ReadyCheckItem] `json:"checks"`
	} `json:"body"`
}

// HostinfoOutput is the response body envelope for GET PathV1Hostinfo.
type HostinfoOutput struct {
	Body hostinfo.Report `json:"body"`
}

type DiagnosticsOutput struct {
	Body DiagnosticsBody `json:"body"`
}

type DiagnosticsBody struct {
	dixdiag.Snapshot
	Runtime RuntimeDiagnostics `json:"runtime"`
}

type RuntimeDiagnostics struct {
	Registered int                             `json:"registered"`
	Available  int                             `json:"available"`
	Providers  *list.List[RuntimeProviderItem] `json:"providers"`
}

type RuntimeProviderItem struct {
	Kind       deployv1.RuntimeKind       `json:"kind"`
	Policy     orchruntime.ProviderPolicy `json:"policy"`
	Available  bool                       `json:"available"`
	Registered bool                       `json:"registered"`
	Status     string                     `json:"status"`
	Reason     string                     `json:"reason,omitempty"`
}

type ListRuntimeProvidersOutput struct {
	Body struct {
		Items *list.List[RuntimeProviderItem] `json:"items"`
	} `json:"body"`
}

// WorkloadItem is the public API representation of a runtime registry record.
type WorkloadItem struct {
	Name      string    `json:"name"`
	Node      string    `json:"node,omitempty"`
	Runtime   string    `json:"runtime"`
	Artifact  string    `json:"artifact"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListWorkloadsOutput is the response body envelope for GET PathV1Workloads.
type ListWorkloadsOutput struct {
	Body struct {
		Items *list.List[WorkloadItem] `json:"items"`
	} `json:"body"`
}

// AssignmentItem is the public API representation of a scheduler assignment.
type AssignmentItem struct {
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

// ListAssignmentsOutput is the response body envelope for GET PathV1Assignments.
type ListAssignmentsOutput struct {
	Body struct {
		Items *list.List[AssignmentItem] `json:"items"`
	} `json:"body"`
}

type AppItem struct {
	Name               string    `json:"name"`
	Namespace          string    `json:"namespace"`
	Status             string    `json:"status"`
	DesiredGeneration  string    `json:"desiredGeneration,omitempty"`
	ObservedGeneration string    `json:"observedGeneration,omitempty"`
	DesiredWorkloads   int       `json:"desiredWorkloads"`
	Running            int       `json:"running"`
	Stopped            int       `json:"stopped"`
	Failed             int       `json:"failed"`
	Pending            int       `json:"pending"`
	LastTransitionAt   time.Time `json:"lastTransitionAt,omitzero"`
	LastError          string    `json:"lastError,omitempty"`
}

type AppWorkloadItem struct {
	Name       string                `json:"name"`
	Kind       deployv1.WorkloadKind `json:"kind"`
	Runtime    deployv1.RuntimeKind  `json:"runtime"`
	Node       string                `json:"node,omitempty"`
	Artifact   string                `json:"artifact,omitempty"`
	Status     string                `json:"status"`
	Generation string                `json:"generation,omitempty"`
	Error      string                `json:"error,omitempty"`
	UpdatedAt  time.Time             `json:"updatedAt,omitzero"`
}

type AppDetailItem struct {
	AppItem
	Metadata  deployv1.Metadata           `json:"metadata"`
	Workloads *list.List[AppWorkloadItem] `json:"workloads"`
}

type ListAppsOutput struct {
	Body struct {
		Items *list.List[AppItem] `json:"items"`
	} `json:"body"`
}

type GetAppInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

type GetAppOutput struct {
	Body AppDetailItem `json:"body"`
}

type WorkloadRuntimeInput struct {
	Namespace string `path:"namespace"`
	App       string `path:"app"`
	Workload  string `path:"workload"`
}

type WorkloadLogsInput struct {
	Namespace string `path:"namespace"`
	App       string `path:"app"`
	Workload  string `path:"workload"`
	Tail      int    `query:"tail,omitempty"`
}

type WorkloadRuntimeStatusOutput struct {
	Body runtimeinfo.Status `json:"body"`
}

type WorkloadLogsOutput struct {
	Body runtimeinfo.LogResult `json:"body"`
}

// DeployInput is the request body envelope for POST PathV1Deploy.
type DeployInput struct {
	Body deployv1.App `json:"body"`
}

// DeploySourceInput is the request body envelope for POST PathV1DeploySource.
type DeploySourceInput struct {
	Body struct {
		// VirtualPath determines parsing (.orch uses plano; .yml/.yaml JSON-like YAML uses v1alpha1), e.g. "app.orch".
		VirtualPath string `json:"virtualPath"`
		Source      string `json:"source"`
	} `json:"body"`
}

// DeployOutput is the response body envelope for POST PathV1Deploy.
type DeployOutput struct {
	Body struct {
		Accepted  bool   `json:"accepted"`
		App       string `json:"app"`
		Workloads int    `json:"workloads"`
	} `json:"body"`
}

type DeleteDeployInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

type DeleteDeployOutput struct {
	Body struct {
		Accepted  bool   `json:"accepted"`
		App       string `json:"app"`
		Namespace string `json:"namespace"`
		Status    string `json:"status"`
	} `json:"body"`
}

type StopDeployInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

// DeployActionBody describes the accepted app action result.
type DeployActionBody struct {
	Accepted  bool   `json:"accepted"`
	App       string `json:"app"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
}

type StopDeployOutput struct {
	Body DeployActionBody `json:"body"`
}

type StartDeployInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

type StartDeployOutput struct {
	Body DeployActionBody `json:"body"`
}

type RestartDeployInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

type RestartDeployOutput struct {
	Body DeployActionBody `json:"body"`
}

type DeployOperationInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
	Body      struct {
		TargetNode string   `json:"targetNode,omitempty"`
		Workloads  []string `json:"workloads,omitempty"`
	} `json:"body"`
}

type DeployOperationOutput struct {
	Body struct {
		Accepted   bool   `json:"accepted"`
		Operation  string `json:"operation"`
		App        string `json:"app"`
		Namespace  string `json:"namespace"`
		TargetNode string `json:"targetNode,omitempty"`
		Workloads  int    `json:"workloads"`
		Moved      int    `json:"moved"`
		Status     string `json:"status"`
	} `json:"body"`
}
