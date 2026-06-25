package api

import (
	"context"

	"github.com/arcgolabs/httpx"

	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workerapi"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// WorkerDeployEndpoint serves POST /api/v1/worker/deploy.
type WorkerDeployEndpoint struct {
	tasks            *task.Service
	openAPIAuthApply bool
}

func NewWorkerDeployEndpoint(tasks *task.Service, openAPIAuthApply bool) *WorkerDeployEndpoint {
	return &WorkerDeployEndpoint{tasks: tasks, openAPIAuthApply: openAPIAuthApply}
}

func (e *WorkerDeployEndpoint) EndpointSpec() httpx.EndpointSpec {
	spec := httpx.EndpointSpec{
		Prefix:      "/v1/worker/deploy",
		Description: "Execute a workload assigned to this worker node.",
		Tags:        httpx.Tags("worker"),
	}
	if e.openAPIAuthApply {
		spec.Security = httpx.SecurityRequirements(httpx.SecurityRequirement("bearerAuth"))
	}
	return spec
}

func (e *WorkerDeployEndpoint) Register(r httpx.Registrar) {
	httpx.MustGroupPost(r.Scope(), "", e.handle, OpenAPIMeta([]string{"worker"}, "deployWorkerWorkload",
		"Execute assigned workload on this worker",
		"Internal worker endpoint used by scheduler dispatch. It starts the assigned workload on the local runtime and does not mutate Raft desired state."))
}

func (e *WorkerDeployEndpoint) handle(ctx context.Context, in *workerapi.DeployWorkloadInput) (*workerapi.DeployWorkloadOutput, error) {
	result, err := e.tasks.DeployWorkerWorkload(ctx, in.Body.Metadata, in.Body.Workload, in.Body.Node)
	if err != nil {
		return nil, oopsx.B("api", "worker").Wrapf(err, "deploy worker workload")
	}
	out := &workerapi.DeployWorkloadOutput{}
	out.Body.Accepted = true
	out.Body.Node = in.Body.Node
	out.Body.Address = result.Address
	out.Body.Status = "running"
	out.Body.Workload = in.Body.Workload.Name
	return out, nil
}
