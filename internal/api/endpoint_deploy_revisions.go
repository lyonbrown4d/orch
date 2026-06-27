package api

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/httpx"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// DeployRevisionsEndpoint serves GET /api/v1/deploy/{namespace}/{name}/revisions.
type DeployRevisionsEndpoint struct {
	tasks *task.Service
}

func NewDeployRevisionsEndpoint(tasks *task.Service) *DeployRevisionsEndpoint {
	return &DeployRevisionsEndpoint{tasks: tasks}
}

func (e *DeployRevisionsEndpoint) EndpointSpec() httpx.EndpointSpec {
	return httpx.EndpointSpec{
		Prefix:      "/v1/deploy/{namespace}/{name}/revisions",
		Description: "List previous desired app revisions stored for rollback.",
		Tags:        httpx.Tags("deploy"),
	}
}

func (e *DeployRevisionsEndpoint) Register(r httpx.Registrar) {
	httpx.MustGroupGet(r.Scope(), "", e.handle, OpenAPIMeta([]string{"deploy"}, "listDeployAppRevisions",
		"List deploy app revisions",
		"Returns previous desired app revisions stored in Raft for rollback. The newest previous revision is listed first."))
}

func (e *DeployRevisionsEndpoint) handle(_ context.Context, in *ListAppRevisionsInput) (*ListAppRevisionsOutput, error) {
	meta := deployv1.Metadata{Name: strings.TrimSpace(in.Name), Namespace: strings.TrimSpace(in.Namespace)}
	if meta.Name == "" {
		return nil, oopsx.B("api").Errorf("app name is required")
	}
	if e == nil || e.tasks == nil {
		return nil, oopsx.B("api").Errorf("task service unavailable")
	}
	revisions, ok := e.tasks.ListAppRevisions(meta)
	if !ok {
		return nil, oopsx.B("api").Errorf("app %s/%s not found", workloadmeta.NamespaceOrDefault(meta.Namespace), meta.Name)
	}
	out := &ListAppRevisionsOutput{}
	out.Body.Items = list.MapList(revisions, func(_ int, revision task.AppRevisionView) AppRevisionItem {
		return AppRevisionItem{
			Generation: revision.Generation,
			Metadata:   revision.Metadata,
			Workloads:  revision.Workloads,
			App:        revision.App,
		}
	})
	return out, nil
}
