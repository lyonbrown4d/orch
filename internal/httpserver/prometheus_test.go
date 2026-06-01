package httpserver

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	prom "github.com/prometheus/client_golang/prometheus"

	"github.com/lyonbrown4d/orch/internal/config"
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
	attachFiberPrometheus(app, cfg, obs)
	app.Get("/hello", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	if _, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/hello", nil), fiber.TestConfig{Timeout: 0}); err != nil {
		t.Fatalf("GET /hello: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/metrics", nil), fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

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
