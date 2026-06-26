package ingress

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func deployAppSortKey(m deployv1.Metadata) string {
	return strings.TrimSpace(m.Namespace) + "/" + strings.TrimSpace(m.Name)
}

// workloadIPv4Lookup is implemented by *dnssvc.Service.
type workloadIPv4Lookup interface {
	LookupWorkloadIPv4(namespace, workloadName string) (string, bool)
}

type workloadAssignmentLookup interface {
	GetWorkloadAssignment(key string) (workloadmeta.Assignment, bool)
}

// IngressCompileOptions controls deploy-document ingress route compilation.
type IngressCompileOptions struct {
	DNS         workloadIPv4Lookup
	Assignments workloadAssignmentLookup
	Cluster     config.ClusterConfig
	Ingress     config.IngressConfig
	LocalNodeID string
	Log         *slog.Logger
}

type ingressRouteTarget struct {
	upstream    string
	stripPrefix string
}

// CompileIngressRoutesFromDeploy flattens app.ingresses into config routes pointing at workload
// container IPs from dnssvc (HTTP endpoints only). Apps are ordered by namespace/name; first match wins.
func CompileIngressRoutesFromDeploy(apps *list.List[deployv1.App], dns workloadIPv4Lookup, log *slog.Logger) *list.List[config.IngressRoute] {
	return CompileIngressRoutesFromDeployWithOptions(apps, IngressCompileOptions{DNS: dns, Log: log})
}

// CompileIngressRoutesFromDeployWithOptions flattens app.ingresses into data-plane routes. It forwards
// non-local running assignments through the remote node's ingress listener and uses local DNS otherwise.
func CompileIngressRoutesFromDeployWithOptions(apps *list.List[deployv1.App], opts IngressCompileOptions) *list.List[config.IngressRoute] {
	if apps == nil || apps.Len() == 0 {
		return list.NewList[config.IngressRoute]()
	}
	out := list.NewList[config.IngressRoute]()
	orderedApps(apps).Range(func(_ int, app deployv1.App) bool {
		compileAppIngressRoutes(out, app, opts)
		return true
	})
	return out
}

func orderedApps(apps *list.List[deployv1.App]) *list.List[deployv1.App] {
	keys, byKey := deployAppsBySortKey(apps)
	out := list.NewListWithCapacity[deployv1.App](keys.Len())
	keys.Range(func(_ int, k string) bool {
		app, _ := byKey.Get(k)
		out.Add(app)
		return true
	})
	return out
}

func deployAppsBySortKey(apps *list.List[deployv1.App]) (*list.List[string], *mapping.Map[string, deployv1.App]) {
	keys := list.NewListWithCapacity[string](apps.Len())
	byKey := mapping.NewMapWithCapacity[string, deployv1.App](apps.Len())
	apps.Range(func(_ int, app deployv1.App) bool {
		if strings.TrimSpace(app.Metadata.Name) != "" {
			addDeployAppSortKey(keys, byKey, app)
		}
		return true
	})
	keys.Sort(strings.Compare)
	return keys, byKey
}

func addDeployAppSortKey(keys *list.List[string], byKey *mapping.Map[string, deployv1.App], app deployv1.App) {
	key := deployAppSortKey(app.Metadata)
	if _, have := byKey.Get(key); !have {
		keys.Add(key)
	}
	byKey.Set(key, app)
}

func workloadMap(app deployv1.App) *mapping.Map[string, deployv1.Workload] {
	workloads := app.WorkloadList()
	out := mapping.NewMapWithCapacity[string, deployv1.Workload](workloads.Len())
	workloads.Range(func(_ int, w deployv1.Workload) bool {
		out.Set(strings.TrimSpace(w.Name), w)
		return true
	})
	return out
}

func compileAppIngressRoutes(out *list.List[config.IngressRoute], app deployv1.App, opts IngressCompileOptions) {
	ctx := ingressCompileContext{
		app:       app,
		namespace: strings.TrimSpace(app.Metadata.Namespace),
		workloads: workloadMap(app),
		opts:      opts,
	}
	app.IngressList().Range(func(_ int, ing deployv1.Ingress) bool {
		ing.RouteList().Range(func(_ int, route deployv1.IngressRoute) bool {
			if compiled, ok := ctx.compileRoute(route); ok {
				out.Add(compiled)
			}
			return true
		})
		return true
	})
}

type ingressCompileContext struct {
	app       deployv1.App
	namespace string
	workloads *mapping.Map[string, deployv1.Workload]
	opts      IngressCompileOptions
}

func (c ingressCompileContext) compileRoute(route deployv1.IngressRoute) (config.IngressRoute, bool) {
	path, ok := ingressPath(route.Path)
	if !ok {
		return config.IngressRoute{}, false
	}
	workloadName := strings.TrimSpace(route.Backend.Workload)
	endpointName := strings.TrimSpace(route.Backend.Endpoint)
	workload, ok := c.workload(workloadName)
	if !ok {
		return config.IngressRoute{}, false
	}
	port, ok := c.endpointPort(workload, workloadName, endpointName)
	if !ok {
		return config.IngressRoute{}, false
	}
	target, ok := c.workloadTarget(workloadName, port)
	if !ok {
		return config.IngressRoute{}, false
	}
	return config.IngressRoute{PathPrefix: path, Upstream: target.upstream, StripPrefix: target.stripPrefix}, true
}

func ingressPath(raw string) (string, bool) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", false
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, true
}

func (c ingressCompileContext) workload(name string) (deployv1.Workload, bool) {
	workload, ok := c.workloads.Get(name)
	if ok {
		return workload, true
	}
	if c.opts.Log != nil {
		c.opts.Log.Warn("ingress route skipped: unknown workload",
			"app", c.app.Metadata.Name, "namespace", c.app.Metadata.Namespace, "workload", name)
	}
	return deployv1.Workload{}, false
}

func (c ingressCompileContext) endpointPort(workload deployv1.Workload, workloadName, endpointName string) (int, bool) {
	port, ok := endpointHTTPPort(workload, endpointName)
	if ok {
		return port, true
	}
	if c.opts.Log != nil {
		c.opts.Log.Warn("ingress route skipped: missing or non-http endpoint",
			"app", c.app.Metadata.Name, "workload", workloadName, "endpoint", endpointName)
	}
	return 0, false
}

func (c ingressCompileContext) workloadTarget(workloadName string, port int) (ingressRouteTarget, bool) {
	if upstream, ok := c.remoteIngressUpstream(workloadName); ok {
		return ingressRouteTarget{upstream: upstream, stripPrefix: "/"}, true
	}
	if ip, ok := c.localWorkloadUpstream(workloadName, port); ok {
		return ingressRouteTarget{upstream: ip}, true
	}
	c.logDeferred(workloadName)
	return ingressRouteTarget{}, false
}

func (c ingressCompileContext) localWorkloadUpstream(workloadName string, port int) (string, bool) {
	if c.opts.DNS == nil {
		return "", false
	}
	ip, ok := c.opts.DNS.LookupWorkloadIPv4(c.namespace, workloadName)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("http://%s:%d", ip, port), true
}

func (c ingressCompileContext) remoteIngressUpstream(workloadName string) (string, bool) {
	assignment, ok := c.runningAssignment(workloadName)
	if !ok {
		return "", false
	}
	if assignment.Node == strings.TrimSpace(c.opts.LocalNodeID) {
		return "", false
	}
	return remoteIngressBaseURL(c.opts.Cluster, c.opts.Ingress, assignment.Node)
}

func (c ingressCompileContext) runningAssignment(workloadName string) (workloadmeta.Assignment, bool) {
	if c.opts.Assignments == nil {
		return workloadmeta.Assignment{}, false
	}
	assignment, ok := c.opts.Assignments.GetWorkloadAssignment(workloadmeta.AssignmentKey(c.app.Metadata, workloadName))
	if !ok || assignment.Status != workloadmeta.AssignmentStatusRunning || strings.TrimSpace(assignment.Node) == "" {
		return workloadmeta.Assignment{}, false
	}
	assignment.Node = strings.TrimSpace(assignment.Node)
	return assignment, true
}

func (c ingressCompileContext) logDeferred(workloadName string) {
	if c.opts.Log == nil {
		return
	}
	c.opts.Log.Debug("ingress route deferred: workload has no local dns or remote ingress target",
		"app", c.app.Metadata.Name, "workload", workloadName)
}
