package httpserver

import (
	"strings"

	fiberprometheus "github.com/gofiber/contrib/v3/prometheus"
	"github.com/gofiber/fiber/v3"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/observability"
)

// attachFiberPrometheus registers HTTP middleware and the scrape route on the shared Prometheus
// registry from observability, so orch_* application metrics and http_fiber_* metrics share one
// endpoint.
//
// Metrics follow the upstream fiberprometheus middleware behavior.
func attachFiberPrometheus(app *fiber.App, cfg config.Config, obs *observability.Service) {
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
		Service:                 serviceName,
		Namespace:               "http",
		Subsystem:               "fiber",
		Registerer:              reg,
		Gatherer:                reg,
		DisableGoCollector:      true,
		DisableProcessCollector: true,
		SkipURIs:                []string{path},
		Next: func(c fiber.Ctx) bool {
			return normalizeHTTPPath(c.Path()) == path
		},
	})
	app.Use(handler)
	app.Use(path, handler)
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
