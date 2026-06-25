package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	prom "github.com/prometheus/client_golang/prometheus"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/httpserver"
	"github.com/lyonbrown4d/orch/internal/observability"
)

func TestAttachFiberPrometheusRecordsHTTPMetrics(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Observability.Prometheus.Path = "/metrics"

	obs, err := observability.New(cfg, prom.NewRegistry(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new observability: %v", err)
	}

	app := fiber.New()
	httpserver.AttachFiberPrometheus(app, cfg, obs)
	app.Get("/hello", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	helloReq := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/hello", http.NoBody)
	helloResp, testErr := app.Test(helloReq, fiber.TestConfig{Timeout: 0})
	if testErr != nil {
		t.Fatalf("GET /hello: %v", testErr)
	}
	if closeErr := helloResp.Body.Close(); closeErr != nil {
		t.Fatalf("close hello body: %v", closeErr)
	}

	metricsReq := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/metrics", http.NoBody)
	resp, err := app.Test(metricsReq, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close metrics body: %v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}

	metrics := string(body)
	if !strings.Contains(metrics, `http_fiber_requests_total{method="GET",path="/hello",service="orch",status_code="204"}`) {
		t.Fatalf("expected /hello request metric, got:\n%s", metrics)
	}
	if strings.Contains(metrics, `path="/metrics"`) {
		t.Fatalf("expected /metrics path to be skipped, got:\n%s", metrics)
	}
}
