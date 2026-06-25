package ingress_test

import (
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/ingress"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func testHTTPHandler(t *testing.T, routes *list.List[config.IngressRoute]) http.Handler {
	t.Helper()
	handler, err := ingress.NewHTTPHandler(routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestNewIngressHTTPHandler_proxyPathRewrite(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeTestResponse(t, w, r.URL.Path)
	}))
	t.Cleanup(upstream.Close)

	handler := testHTTPHandler(t, list.NewList(
		config.IngressRoute{PathPrefix: "/api", Upstream: upstream.URL},
	))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1/api/v1/hello", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer closeResponseBody(t, resp)
	body := readResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if string(body) != "/v1/hello" {
		t.Fatalf("upstream saw path: got %q want %q", body, "/v1/hello")
	}
}

func TestNewIngressHTTPHandler_noRouteNotFound(t *testing.T) {
	t.Parallel()

	handler := testHTTPHandler(t, list.NewList(
		config.IngressRoute{PathPrefix: "/api", Upstream: "http://127.0.0.1:1"},
	))
	req := newTestRequest(t, "http://127.0.0.1/other")
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer closeResponseBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestNewIngressHTTPHandler_pathPrefixBoundary(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream should not receive %s", r.URL.Path)
	}))
	t.Cleanup(upstream.Close)

	handler := testHTTPHandler(t, list.NewList(
		config.IngressRoute{PathPrefix: "/api", Upstream: upstream.URL},
	))
	req := newTestRequest(t, "http://127.0.0.1/api2")
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer closeResponseBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestNewIngressHTTPHandler_placeholderNoRoutes(t *testing.T) {
	t.Parallel()
	handler := testHTTPHandler(t, nil)
	req := newTestRequest(t, "http://127.0.0.1/")
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer closeResponseBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestNewIngressHTTPHandler_roundRobinDistributes(t *testing.T) {
	t.Parallel()

	var hitsA, hitsB int
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA++
		writeTestResponse(t, w, "a")
	}))
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB++
		writeTestResponse(t, w, "b")
	}))
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)

	handler := testHTTPHandler(t, list.NewList(
		config.IngressRoute{
			PathPrefix: "/p",
			Upstreams:  []string{srvA.URL, srvB.URL},
			LB:         "round_robin",
		},
	))

	for range 8 {
		req := newTestRequest(t, "http://127.0.0.1/p/x")
		req.Host = "127.0.0.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status: %d", resp.StatusCode)
		}
		closeResponseBody(t, resp)
	}

	if hitsA < 1 || hitsB < 1 {
		t.Fatalf("expected both upstreams to receive traffic, got hitsA=%d hitsB=%d", hitsA, hitsB)
	}
	if hitsA+hitsB != 8 {
		t.Fatalf("hitsA+hitsB=%d want 8", hitsA+hitsB)
	}
}

func writeTestResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, html.EscapeString(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func newTestRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func readResponseBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return body
}

func closeResponseBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}

func TestIngressRouteUpstreamEndpoints(t *testing.T) {
	t.Parallel()
	r := config.IngressRoute{Upstream: "http://a"}
	if got := r.UpstreamEndpoints(); got.Len() != 1 {
		t.Fatalf("got %#v", got)
	} else if first, _ := got.Get(0); first != "http://a" {
		t.Fatalf("got %#v", got.Values())
	}
	r2 := config.IngressRoute{Upstreams: []string{"http://a", "http://b"}, Upstream: "http://ignored"}
	got := r2.UpstreamEndpoints()
	if got.Len() != 2 {
		t.Fatalf("got %#v", got.Values())
	}
}

func TestIngressRouteLBPolicy(t *testing.T) {
	t.Parallel()
	if got := (&config.IngressRoute{}).LBPolicy(); got != "round_robin" {
		t.Fatal(got)
	}
	if got := (&config.IngressRoute{LB: "ROUND_ROBIN"}).LBPolicy(); got != "round_robin" {
		t.Fatal(got)
	}
}

// mapDNS mimics dnssvc workloadRecordKey lookup (lowercase namespace/workload).
type mapDNS map[string]string

func (m mapDNS) LookupWorkloadIPv4(namespace, workloadName string) (string, bool) {
	if m == nil {
		return "", false
	}
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	key := strings.ToLower(ns) + "/" + strings.ToLower(strings.TrimSpace(workloadName))
	ip, ok := m[key]
	return ip, ok
}

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

type mapAssignments map[string]workloadmeta.Assignment

func (m mapAssignments) GetWorkloadAssignment(key string) (workloadmeta.Assignment, bool) {
	assignment, ok := m[key]
	return assignment, ok
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

func TestNewIngressHTTPHandlerRemoteForwardPreservesPrefix(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestResponse(t, w, r.URL.Path)
	}))
	t.Cleanup(upstream.Close)

	handler := testHTTPHandler(t, list.NewList(
		config.IngressRoute{PathPrefix: "/api", Upstream: upstream.URL, StripPrefix: "/"},
	))
	req := newTestRequest(t, "http://127.0.0.1/api/v1/hello")
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer closeResponseBody(t, resp)
	body := readResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if string(body) != "/api/v1/hello" {
		t.Fatalf("upstream saw path: got %q want %q", body, "/api/v1/hello")
	}
}
