package dnssvc

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type Service struct {
	logger          *slog.Logger
	cfg             config.DNSConfig
	store           *dnsserver.BboltStore
	server          *dnsserver.Server
	workloadRecords *mapping.ShardedConcurrentMap[string, dnsserver.Record]
	assignments     workloadAssignmentLookup
	started         atomic.Bool
}

func New(cfg config.Config, logger *slog.Logger) *Service {
	return NewWithAssignmentLookup(cfg, logger, nil)
}

func NewWithAssignmentLookup(cfg config.Config, logger *slog.Logger, assignments workloadAssignmentLookup) *Service {
	return &Service{
		logger:          logger,
		cfg:             cfg.DNS,
		workloadRecords: mapping.NewShardedConcurrentMap[string, dnsserver.Record](0, mapping.HashString),
		assignments:     assignments,
	}
}

func (s *Service) Start(ctx context.Context) error {
	if !s.cfg.Enabled {
		s.logger.Info("dns service disabled by config")
		return nil
	}
	if s.started.Load() {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.cfg.Data.Path), 0o750); err != nil {
		return oopsx.B("dns").Wrapf(err, "mkdir dns data dir")
	}
	store, err := dnsserver.OpenBboltStore(s.cfg.Data.Path, s.logger)
	if err != nil {
		return oopsx.B("dns").Wrapf(err, "open dns store")
	}
	server, upstreams := s.newServer(ctx, store)
	if err := server.Start(ctx); err != nil {
		s.closeStoreAfterStartFailure(store)
		return oopsx.B("dns").Wrapf(err, "start dns server")
	}

	s.store = store
	s.server = server
	s.started.Store(true)
	if err := s.seedZoneRecord(ctx); err != nil {
		return err
	}
	s.logStarted(upstreams)
	return nil
}

func (s *Service) newServer(ctx context.Context, store *dnsserver.BboltStore) (*dnsserver.Server, *list.List[string]) {
	resolver := dnsserver.NewResolver(store, dnsserver.WithResolverLogger(s.logger))
	upstreams := s.cfg.WorkloadUpstreamList(ctx)
	return dnsserver.NewServerWithResolver(
		dnsserver.Config{Listen: s.cfg.Listen},
		resolver,
		s.serverOptions(resolver, upstreams)...,
	), upstreams
}

func (s *Service) serverOptions(resolver *dnsserver.Resolver, upstreams *list.List[string]) []dnsserver.Option {
	return []dnsserver.Option{
		dnsserver.WithLogger(s.logger),
		dnsserver.WithHandler(newForwardingHandler(resolver, upstreams, s.logger, s.cfg.ZoneName(), s)),
	}
}

func (s *Service) closeStoreAfterStartFailure(store *dnsserver.BboltStore) {
	if closeErr := store.Close(); closeErr != nil {
		s.logger.Warn("close dns store after start failure", "error", closeErr)
	}
}

func (s *Service) seedZoneRecord(ctx context.Context) error {
	zone := dnsZoneName(s.cfg)
	if err := s.store.SaveRecord(ctx, dnsserver.Record{
		Zone: zone,
		Name: zone,
		TTL:  60,
		Type: dns.TypeA,
		Data: "127.0.0.1",
	}); err != nil {
		return oopsx.B("dns").Wrapf(err, "seed zone record")
	}
	return nil
}

func (s *Service) logStarted(upstreams *list.List[string]) {
	s.logger.Info("dns service started", "listen", s.cfg.Listen, "udp", s.server.UDPAddr(), "tcp", s.server.TCPAddr())
	if upstreams.Len() > 0 {
		s.logger.Info("dns workload upstreams configured", "upstream", upstreams.Values())
	}
	if ns, ok := s.WorkloadNameserver(); ok {
		s.logger.Info("dns workload resolver configured",
			"nameserver", ns,
			"search", s.WorkloadSearchDomains("default").Values(),
		)
	}
}

func (s *Service) Stop(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}

	if s.server != nil {
		if err := s.server.Stop(ctx); err != nil {
			return oopsx.B("dns").Wrapf(err, "stop dns server")
		}
	}
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			return oopsx.B("dns").Wrapf(err, "close dns store")
		}
	}

	s.started.Store(false)
	s.logger.Info("dns service stopped")
	return nil
}

// UDPAddr returns the active UDP listener address, or an empty string when DNS is not running.
func (s *Service) UDPAddr() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.UDPAddr()
}
