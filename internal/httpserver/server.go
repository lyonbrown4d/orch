package httpserver

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	authhttp "github.com/arcgolabs/authx/http"
	"github.com/arcgolabs/httpx"
	"github.com/gofiber/fiber/v3"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/observability"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type Server struct {
	logger   *slog.Logger
	addr     string
	runtime  httpx.ServerRuntime
	fiberApp *fiber.App
}

func New(cfg config.Config, logger *slog.Logger, guard *authhttp.Guard, obs *observability.Service) (*Server, error) {
	fiberApp, rt := newFiberAppAndRuntime(cfg, logger, guard)
	attachFiberPrometheus(fiberApp, cfg, obs)

	return &Server{
		logger:   logger,
		addr:     cfg.HTTP.Addr,
		runtime:  rt,
		fiberApp: fiberApp,
	}, nil
}

// LogRegisteredRoutes logs Fiber routes (methods + paths) after handlers are registered.
// Passing true to [fiber.App.GetRoutes] skips middleware-only registrations.
func (s *Server) LogRegisteredRoutes() {
	if s == nil || s.fiberApp == nil || s.logger == nil {
		return
	}
	routes := s.fiberApp.GetRoutes(true)
	slices.SortFunc(routes, func(a, b fiber.Route) int {
		if c := strings.Compare(a.Method, b.Method); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	s.logger.Info("http routes registered", "count", len(routes))
	for i := range routes {
		r := routes[i]
		if r.Name != "" {
			s.logger.Info("http route", "method", r.Method, "path", r.Path, "name", r.Name)
		} else {
			s.logger.Info("http route", "method", r.Method, "path", r.Path)
		}
	}
}

func (s *Server) Runtime() httpx.ServerRuntime {
	return s.runtime
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("http server starting", "addr", s.addr)
	if host := config.FixLoopbackHost(s.addr); host != "" {
		base := "http://" + host
		s.logger.Info("http openapi docs",
			"openapi_spec", base+OpenAPIJSONPath,
			"swagger_ui", base+OpenAPIDocsPath,
		)
	}
	go func() {
		if err := s.runtime.ListenAndServeContext(ctx, s.addr); err != nil {
			s.logger.Error("http server stopped with error", "error", err)
		}
	}()
	return nil
}

func (s *Server) Stop(_ context.Context) error {
	s.logger.Info("http server stopping")
	if err := s.runtime.Shutdown(); err != nil {
		return oopsx.B("http").Wrapf(err, "shutdown httpx runtime")
	}
	return nil
}
