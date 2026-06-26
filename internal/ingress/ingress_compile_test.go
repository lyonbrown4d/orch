package ingress_test

import (
	"testing"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/ingress"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestCompileIngressRoutesFromDeploy(t *testing.T) {
	t.Parallel()
	apps := list.NewList(deployv1.App{
		Metadata: deployv1.Metadata{Name: "a", Namespace: "ns"},
		Workloads: []deployv1.Workload{{
			Name: "web",
			Endpoints: []deployv1.Endpoint{{
				Name: "http", Port: 8080, Protocol: deployv1.ProtoHTTP,
			}},
		}},
		Ingresses: []deployv1.Ingress{{
			Routes: []deployv1.IngressRoute{{
				Path:    "/api",
				Backend: deployv1.EndpointRef{Workload: "web", Endpoint: "http"},
			}},
		}},
	})
	dns := mapDNS{"ns/web": "10.0.0.2"}
	got := ingress.CompileIngressRoutesFromDeploy(apps, dns, nil)
	route, ok := got.Get(0)
	if got.Len() != 1 || !ok || route.PathPrefix != "/api" || route.Upstream != "http://10.0.0.2:8080" {
		t.Fatalf("got %#v", got.Values())
	}
}

func TestCompileIngressRoutesFromDeployRemoteAssignment(t *testing.T) {
	t.Parallel()
	app := deployv1.App{
		Metadata: deployv1.Metadata{Name: "a", Namespace: "ns"},
		Workloads: []deployv1.Workload{{
			Name: "web",
			Endpoints: []deployv1.Endpoint{{
				Name: "http", Port: 8080, Protocol: deployv1.ProtoHTTP,
			}},
		}},
		Ingresses: []deployv1.Ingress{{
			Routes: []deployv1.IngressRoute{{
				Path:    "/api",
				Backend: deployv1.EndpointRef{Workload: "web", Endpoint: "http"},
			}},
		}},
	}
	assignments := mapAssignments{
		workloadmeta.AssignmentKey(app.Metadata, "web"): {
			Node:   "node-b",
			Status: workloadmeta.AssignmentStatusRunning,
		},
	}

	got := ingress.CompileIngressRoutesFromDeployWithOptions(list.NewList(app), ingress.IngressCompileOptions{
		DNS:         mapDNS{},
		Assignments: assignments,
		Cluster:     config.ClusterConfig{Nodes: map[string]string{"node-b": "http://node-b:17443"}},
		Ingress:     config.IngressConfig{Listen: []string{":18080"}},
		LocalNodeID: "node-a",
	})

	route, ok := got.Get(0)
	if got.Len() != 1 || !ok || route.PathPrefix != "/api" || route.Upstream != "http://node-b:18080" || route.StripPrefix != "/" {
		t.Fatalf("got %#v", got.Values())
	}
}

func TestCompileIngressRoutesFromDeployRemoteAssignmentPrefersIngressOverDNS(t *testing.T) {
	t.Parallel()
	app := deployv1.App{
		Metadata: deployv1.Metadata{Name: "a", Namespace: "ns"},
		Workloads: []deployv1.Workload{{
			Name: "web",
			Endpoints: []deployv1.Endpoint{{
				Name: "http", Port: 8080, Protocol: deployv1.ProtoHTTP,
			}},
		}},
		Ingresses: []deployv1.Ingress{{
			Routes: []deployv1.IngressRoute{{
				Path:    "/api",
				Backend: deployv1.EndpointRef{Workload: "web", Endpoint: "http"},
			}},
		}},
	}
	assignments := mapAssignments{
		workloadmeta.AssignmentKey(app.Metadata, "web"): {
			Node:   "node-b",
			Status: workloadmeta.AssignmentStatusRunning,
		},
	}

	got := ingress.CompileIngressRoutesFromDeployWithOptions(list.NewList(app), ingress.IngressCompileOptions{
		DNS:         mapDNS{"ns/web": "172.18.0.2"},
		Assignments: assignments,
		Cluster:     config.ClusterConfig{Nodes: map[string]string{"node-b": "http://node-b:17443"}},
		Ingress:     config.IngressConfig{Listen: []string{":18080"}},
		LocalNodeID: "node-a",
	})

	route, ok := got.Get(0)
	if got.Len() != 1 || !ok || route.Upstream != "http://node-b:18080" || route.StripPrefix != "/" {
		t.Fatalf("got %#v", got.Values())
	}
}
func TestCompileIngressRoutesFromDeployLocalAssignmentWithoutDNSDefers(t *testing.T) {
	t.Parallel()
	app := deployv1.App{
		Metadata: deployv1.Metadata{Name: "a", Namespace: "ns"},
		Workloads: []deployv1.Workload{{
			Name: "web",
			Endpoints: []deployv1.Endpoint{{
				Name: "http", Port: 8080, Protocol: deployv1.ProtoHTTP,
			}},
		}},
		Ingresses: []deployv1.Ingress{{
			Routes: []deployv1.IngressRoute{{
				Path:    "/api",
				Backend: deployv1.EndpointRef{Workload: "web", Endpoint: "http"},
			}},
		}},
	}
	assignments := mapAssignments{
		workloadmeta.AssignmentKey(app.Metadata, "web"): {
			Node:   "node-a",
			Status: workloadmeta.AssignmentStatusRunning,
		},
	}

	got := ingress.CompileIngressRoutesFromDeployWithOptions(list.NewList(app), ingress.IngressCompileOptions{
		DNS:         mapDNS{},
		Assignments: assignments,
		Cluster:     config.ClusterConfig{Nodes: map[string]string{"node-a": "http://node-a:17443"}},
		Ingress:     config.IngressConfig{Listen: []string{":18080"}},
		LocalNodeID: "node-a",
	})

	if got.Len() != 0 {
		t.Fatalf("got %#v", got.Values())
	}
}
