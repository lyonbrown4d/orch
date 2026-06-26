package ingress_test

import (
	"html"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

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

type mapAssignments map[string]workloadmeta.Assignment

func (m mapAssignments) GetWorkloadAssignment(key string) (workloadmeta.Assignment, bool) {
	assignment, ok := m[key]
	return assignment, ok
}
