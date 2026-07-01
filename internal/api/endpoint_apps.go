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

// AppsEndpoint serves GET /api/v1/apps and GET /api/v1/apps/{namespace}/{name}.
type AppsEndpoint struct {
	tasks *task.Service
}

func NewAppsEndpoint(tasks *task.Service) *AppsEndpoint {
	return &AppsEndpoint{tasks: tasks}
}

func (e *AppsEndpoint) EndpointSpec() httpx.EndpointSpec {
	return httpx.EndpointSpec{
		Prefix:      "/v1/apps",
		Description: "Desired app status aggregated from scheduler assignments.",
		Tags:        httpx.Tags("apps"),
	}
}

func (e *AppsEndpoint) Register(r httpx.Registrar) {
	httpx.MustGroupGet(r.Scope(), "", e.list, OpenAPIMeta([]string{"apps"}, "listApps", "List apps",
		"Returns desired apps with aggregated workload assignment status."))
	httpx.MustGroupGet(r.Scope(), "/{namespace}/{name}", e.get, OpenAPIMeta([]string{"apps"}, "getApp", "Get app",
		"Returns one desired app with per-workload assignment status."))
}

func (e *AppsEndpoint) list(_ context.Context, _ *EmptyInput) (*ListAppsOutput, error) {
	out := &ListAppsOutput{}
	out.Body.Items = list.NewList[AppItem]()
	if e != nil && e.tasks != nil {
		out.Body.Items = list.MapList(e.tasks.ListApps(), func(_ int, app task.AppView) AppItem {
			return appItem(app)
		})
	}
	return out, nil
}

func (e *AppsEndpoint) get(_ context.Context, in *GetAppInput) (*GetAppOutput, error) {
	meta := deployv1.Metadata{Name: strings.TrimSpace(in.Name), Namespace: strings.TrimSpace(in.Namespace)}
	if meta.Name == "" {
		return nil, oopsx.B("api").Errorf("app name is required")
	}
	if e == nil || e.tasks == nil {
		return nil, oopsx.B("api").Errorf("task service unavailable")
	}
	app, ok := e.tasks.GetApp(meta)
	if !ok {
		return nil, oopsx.B("api").Errorf("app %s/%s not found", workloadmeta.NamespaceOrDefault(meta.Namespace), meta.Name)
	}
	out := &GetAppOutput{}
	out.Body = appDetailItem(app)
	return out, nil
}

func appItem(app task.AppView) AppItem {
	return AppItem{
		Name:               app.Metadata.Name,
		Namespace:          workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
		Status:             app.Status,
		DesiredGeneration:  app.DesiredGeneration,
		ObservedGeneration: app.ObservedGeneration,
		DesiredWorkloads:   app.DesiredWorkloads,
		Running:            app.Running,
		Stopped:            app.Stopped,
		Failed:             app.Failed,
		Pending:            app.Pending,
		LastTransitionAt:   app.LastTransitionAt,
		LastError:          app.LastError,
		RevisionCount:      app.RevisionCount,
		RollbackAvailable:  app.RollbackAvailable,
		PreviousGeneration: app.PreviousGeneration,
	}
}

func appDetailItem(app task.AppView) AppDetailItem {
	return AppDetailItem{
		AppItem:  appItem(app),
		Metadata: app.Metadata,
		Workloads: list.MapList(app.Workloads, func(_ int, workload task.AppWorkloadView) AppWorkloadItem {
			return AppWorkloadItem{
				Name:       workload.Name,
				Kind:       workload.Kind,
				Runtime:    workload.Runtime,
				Node:       workload.Node,
				Artifact:   workload.Artifact,
				Status:     workload.Status,
				Generation: workload.Generation,
				Error:      workload.Error,
				UpdatedAt:  workload.UpdatedAt,
			}
		}),
	}
}
