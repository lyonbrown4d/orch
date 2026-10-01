package httpserver

import (
	"strings"

	fiberprometheus "github.com/gofiber/contrib/v3/prometheus"
	"github.com/gofiber/fiber/v3"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/observability"
)

// AttachFiberPrometheus registers HTTP middleware and the scrape route on the shared Prometheus
// registry from observability, so orch_* application metrics and http_fiber_* metrics share one
// endpoint.
//
// Metrics follow the upstream fiberprometheus middleware behavior.
func AttachFiberPrometheus(app *fiber.App, cfg config.Config, obs *observability.Service) {
	reg := obs.PrometheusRegistry()
	if reg == nil {
		return
	}

	path := strings.TrimSpace(cfg.Observability.Prometheus.Path)
	if path == "" {
		path = "/metrics"
	}
	path = normalizeHTTPPath(path)

	serviceName := cfg.App.Name
	if serviceName == "" {
		serviceName = "orch"
	}

	handler := fiberprometheus.New(fiberprometheus.Config{
		ServiceName:             serviceName,
		Namespace:               "http",
		Subsystem:               "fiber",
		MetricsPath:             path,
		Registerer:              reg,
		Gatherer:                reg,
		DisableGoCollector:      true,
		DisableProcessCollector: true,
	})
	app.Use(handler)
}

func normalizeHTTPPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}
