package api

import (
	"context"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/httpx"

	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

// AssignmentsEndpoint serves GET /api/v1/assignments.
type AssignmentsEndpoint struct {
	tasks *task.Service
}

func NewAssignmentsEndpoint(tasks *task.Service) *AssignmentsEndpoint {
	return &AssignmentsEndpoint{tasks: tasks}
}

func (e *AssignmentsEndpoint) EndpointSpec() httpx.EndpointSpec {
	return httpx.EndpointSpec{
		Prefix:      "/v1/assignments",
		Description: "Scheduler assignment state replicated through Raft.",
		Tags:        httpx.Tags("scheduler"),
	}
}

func (e *AssignmentsEndpoint) Register(r httpx.Registrar) {
	httpx.MustGroupGet(r.Scope(), "", e.Handle, OpenAPIMeta([]string{"scheduler"}, "listWorkloadAssignments", "List scheduler workload assignments",
		"Sorted workload assignment records including app metadata, workload name, assigned node, runtime, artifact, status, and last error."))
}

// Handle returns the current scheduler assignment view.
func (e *AssignmentsEndpoint) Handle(_ context.Context, _ *EmptyInput) (*ListAssignmentsOutput, error) {
	out := &ListAssignmentsOutput{}
	out.Body.Items = list.NewList[AssignmentItem]()
	if e != nil && e.tasks != nil {
		out.Body.Items = list.MapList(e.tasks.ListWorkloadAssignments(), func(_ int, assignment workloadmeta.Assignment) AssignmentItem {
			return AssignmentItem{
				Key:        assignment.Key,
				Metadata:   assignment.Metadata,
				Workload:   assignment.Workload,
				Node:       assignment.Node,
				Address:    assignment.Address,
				Runtime:    assignment.Runtime,
				Artifact:   assignment.Artifact,
				Status:     assignment.Status,
				Generation: assignment.Generation,
				Error:      assignment.Error,
				UpdatedAt:  assignment.UpdatedAt,
			}
		})
	}
	return out, nil
}
