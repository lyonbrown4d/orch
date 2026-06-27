package api

import (
	"github.com/arcgolabs/httpx"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	"github.com/lyonbrown4d/orch/internal/dixdiag"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/services/registry"
	"github.com/lyonbrown4d/orch/internal/services/task"
)

type openAPIAuthApply bool

type RouteEndpoints struct {
	items []any
}

type systemEndpointGroup struct {
	items []any
}

type workloadEndpointGroup struct {
	items []any
}

type raftEndpointGroup struct {
	items []any
}

type deployEndpointGroup struct {
	items []any
}

type workerEndpointGroup struct {
	items []any
}

// Register wires all HTTP [httpx.Endpoint] modules in one place. Each module owns its Prefix and handlers;
// route paths use "" to bind to that prefix root (see per-type EndpointSpec).
func Register(rt httpx.ServerRuntime, cfg config.Config, registrySvc *registry.Service, taskSvc *task.Service, loaderSvc *loader.Loader, runtimeSvc *orchruntime.Manager, dnsSvc *dnssvc.Service, raftSvc *raftsvc.Service, dixdiagSvcs ...*dixdiag.Service) {
	var dixdiagSvc *dixdiag.Service
	if len(dixdiagSvcs) > 0 {
		dixdiagSvc = dixdiagSvcs[0]
	}
	leader := NewLeaderForwarder(cfg, raftSvc)
	auth := openAPIAuthApply(cfg.Auth.Enabled)
	RegisterEndpoints(rt, newRouteEndpoints(
		newSystemEndpoints(cfg, raftSvc, runtimeSvc, dnsSvc, dixdiagSvc),
		newWorkloadEndpoints(registrySvc, taskSvc),
		newRaftEndpoints(cfg, raftSvc, leader, auth),
		newDeployEndpoints(taskSvc, loaderSvc, leader, auth),
		newWorkerEndpoints(taskSvc, auth),
	))
}

func RegisterEndpoints(rt httpx.ServerRuntime, endpoints RouteEndpoints) {
	rt.RegisterOnly(endpoints.items...)
}

func newOpenAPIAuthApply(cfg config.Config) openAPIAuthApply {
	return openAPIAuthApply(cfg.Auth.Enabled)
}

func newLeaderForwarderProvider(cfg config.Config, raftSvc *raftsvc.Service) *LeaderForwarder {
	return NewLeaderForwarder(cfg, raftSvc)
}

func newRouteEndpoints(system systemEndpointGroup, workloads workloadEndpointGroup, raft raftEndpointGroup, deploy deployEndpointGroup, worker workerEndpointGroup) RouteEndpoints {
	items := make([]any, 0, len(system.items)+len(workloads.items)+len(raft.items)+len(deploy.items)+len(worker.items))
	items = append(items, system.items...)
	items = append(items, workloads.items...)
	items = append(items, raft.items...)
	items = append(items, deploy.items...)
	items = append(items, worker.items...)
	return RouteEndpoints{items: items}
}

func newSystemEndpoints(cfg config.Config, raftSvc *raftsvc.Service, runtimeSvc *orchruntime.Manager, dnsSvc *dnssvc.Service, dixdiagSvc *dixdiag.Service) systemEndpointGroup {
	return systemEndpointGroup{items: []any{
		NewHealthEndpoint(dixdiagSvc),
		NewReadyEndpoint(cfg, raftSvc, runtimeSvc, dixdiagSvc),
		NewDiagnosticsEndpoint(dixdiagSvc, runtimeSvc),
		NewHostinfoEndpoint(),
		NewRuntimeProvidersEndpoint(runtimeSvc),
		NewOrchVPNBootstrapEndpoint(cfg, dnsSvc),
	}}
}

func newWorkloadEndpoints(registrySvc *registry.Service, taskSvc *task.Service) workloadEndpointGroup {
	return workloadEndpointGroup{items: []any{
		NewAppsEndpoint(taskSvc),
		NewWorkloadsEndpoint(registrySvc),
		NewWorkloadRuntimeEndpoint(taskSvc),
		NewAssignmentsEndpoint(taskSvc),
		NewVolumesEndpoint(taskSvc),
	}}
}

func newRaftEndpoints(cfg config.Config, raftSvc *raftsvc.Service, leader *LeaderForwarder, auth openAPIAuthApply) raftEndpointGroup {
	return raftEndpointGroup{items: []any{
		NewRaftStatusEndpoint(cfg, raftSvc, bool(auth)),
		NewRaftMembersEndpoint(raftSvc, leader, bool(auth)),
	}}
}

func newDeployEndpoints(taskSvc *task.Service, loaderSvc *loader.Loader, leader *LeaderForwarder, auth openAPIAuthApply) deployEndpointGroup {
	return deployEndpointGroup{items: []any{
		NewDeployEndpoint(taskSvc, leader, bool(auth)),
		NewStartDeployEndpoint(taskSvc, leader, bool(auth)),
		NewStopDeployEndpoint(taskSvc, leader, bool(auth)),
		NewRestartDeployEndpoint(taskSvc, leader, bool(auth)),
		NewRollbackDeployEndpoint(taskSvc, leader, bool(auth)),
		NewMigrateDeployEndpoint(taskSvc, leader, bool(auth)),
		NewFailoverDeployEndpoint(taskSvc, leader, bool(auth)),
		NewRebalanceDeployEndpoint(taskSvc, leader, bool(auth)),
		NewDeleteDeployEndpoint(taskSvc, leader, bool(auth)),
		NewDeploySourceEndpoint(loaderSvc, taskSvc, leader, bool(auth)),
	}}
}

func newWorkerEndpoints(taskSvc *task.Service, auth openAPIAuthApply) workerEndpointGroup {
	return workerEndpointGroup{items: []any{
		NewWorkerDeployEndpoint(taskSvc, bool(auth)),
		NewWorkerStopEndpoint(taskSvc, bool(auth)),
		NewWorkerStatusEndpoint(taskSvc, bool(auth)),
		NewWorkerLogsEndpoint(taskSvc, bool(auth)),
	}}
}
