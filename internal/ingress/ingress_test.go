package ingress_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/ingress"
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
