package httpserver

import (
	"log/slog"

	authhttp "github.com/arcgolabs/authx/http"
	authfiber "github.com/arcgolabs/authx/http/fiber"
	"github.com/arcgolabs/httpx"
	"github.com/arcgolabs/httpx/adapter"
	adapterfiber "github.com/arcgolabs/httpx/adapter/fiber"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/lyonbrown4d/orch/internal/buildmeta"
	"github.com/lyonbrown4d/orch/internal/config"
)

// newFiberAppAndRuntime wires Fiber + httpx with OpenAPI defaults and optional deploy-route auth.
func newFiberAppAndRuntime(cfg config.Config, logger *slog.Logger, guard *authhttp.Guard) (*fiber.App, httpx.ServerRuntime) {
	fiberApp := fiber.New()
	fiberAdapter := adapterfiber.New(fiberApp, adapter.HumaOptions{
		Title:       "orch API",
		Version:     buildmeta.Version(),
		Description: "Orch control plane API",
		DocsPath:    OpenAPIDocsPath,
		OpenAPIPath: OpenAPIJSONPath,
	})
	if cfg.Auth.Enabled && guard != nil {
		fiberApp.Use("/api/v1/deploy", authfiber.Require(guard))
		fiberApp.Use("/api/v1/worker", authfiber.Require(guard))
	}

	opts := []httpx.ServerOption{
		httpx.WithAdapter(fiberAdapter),
		httpx.WithLogger(logger),
		httpx.WithValidation(),
		httpx.WithBasePath("/api"),
	}
	if cfg.Auth.Enabled {
		opts = append(opts, httpx.WithSecurity(httpx.SecurityOptions{
			Schemes: httpx.SecuritySchemes(map[string]*huma.SecurityScheme{
				"bearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
					Description:  "When orch HTTP auth is enabled, send the same bearer token as the CLI (e.g. Authorization: Bearer <token>).",
				},
			}),
		}))
	}
	rt := httpx.New(opts...)
	return fiberApp, rt
}
