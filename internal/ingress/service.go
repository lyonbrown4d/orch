package ingress

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	valeruntime "github.com/arcgolabs/vale/runtime"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type Service struct {
	logger        *slog.Logger
	cfg           config.IngressConfig
	cluster       config.ClusterConfig
	localNodeID   string
	raft          *raftsvc.Service
	dns           *dnssvc.Service
	dataRoot      string
	mu            sync.Mutex
	servers       []*http.Server
	gateway       *valeruntime.Gateway
	vale          valeFactory
	routeCount    atomic.Int64
	refreshCancel context.CancelFunc
	refreshWG     sync.WaitGroup
	started       bool // guarded by mu
}

func New(cfg config.Config, logger *slog.Logger, raft *raftsvc.Service, dns *dnssvc.Service) *Service {
	return newWithValeFactory(cfg, logger, raft, dns, nodeid.Local{}, newValeFactory())
}

func newWithValeFactory(cfg config.Config, logger *slog.Logger, raft *raftsvc.Service, dns *dnssvc.Service, local nodeid.Local, vale valeFactory) *Service {
	return &Service{
		logger:      logger,
		cfg:         cfg.Ingress,
		cluster:     cfg.Cluster,
		localNodeID: strings.TrimSpace(local.String()),
		raft:        raft,
		dns:         dns,
		vale:        vale,
		dataRoot:    config.DefaultDataRoot(),
	}
}

func (s *Service) refreshRoutes() {
	if !s.cfg.Enabled || s.raft == nil || s.dns == nil {
		return
	}
	apps := s.raft.ListDesiredDeployApps()
	routes := CompileIngressRoutesFromDeployWithOptions(apps, IngressCompileOptions{
		DNS:         s.dns,
		Assignments: s.raft,
		Cluster:     s.cluster,
		Ingress:     s.cfg,
		LocalNodeID: s.localNodeID,
		Log:         s.logger,
	})
	snapshot, routeCount, err := s.vale.build(routes)
	if err != nil {
		s.logger.Warn("ingress routes compile failed", "error", err)
		return
	}
	s.mu.Lock()
	gateway := s.gateway
	if gateway != nil {
		gateway.Swap(snapshot)
		s.routeCount.Store(int64(routeCount))
	}
	s.mu.Unlock()
	log := s.logger.With(slog.String("component", "ingress"))
	log.Info("ingress routes refreshed", "routes", routeCount)
}
func (s *Service) Start(ctx context.Context) error {
	if !s.cfg.Enabled {
		s.logger.Info("ingress disabled by config")
		return nil
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	tlsConf, err := s.startTLSConfig()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	listeners, err := s.openListeners(ctx, tlsConf.value)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	log := s.logger.With(slog.String("component", "ingress"), slog.String("engine", "vale"))
	if err := s.startGatewayServers(listeners.all, log); err != nil {
		s.closeListeners(listeners.all)
		s.mu.Unlock()
		return err
	}
	s.started = true
	s.startRefreshLoop(ctx)
	s.mu.Unlock()

	s.refreshRoutes()
	s.logger.Info("ingress started", "plain_listen", listeners.plain, "tls_listen", listeners.tls)
	return nil
}

type ingressTLSConfig struct {
	value *tls.Config
}

func (s *Service) startTLSConfig() (ingressTLSConfig, error) {
	if !s.cfg.TLS.Enabled {
		return ingressTLSConfig{}, nil
	}
	domains := s.cfg.TLSAutocertDomainList()
	manager, err := newAutocertManager(s.cfg.TLS, domains, s.dataRoot)
	if err != nil {
		return ingressTLSConfig{}, err
	}
	if strings.TrimSpace(s.cfg.TLS.Email) == "" {
		s.logger.Warn("ingress autocert: ingress.tls.email is empty (Let's Encrypt recommends a contact email)")
	}
	s.logger.Info("ingress autocert",
		"domains", domains.Values(),
		"tls_listen", s.cfg.TLSListenAddrList().Values(),
		"staging", s.cfg.TLS.Staging,
	)
	return ingressTLSConfig{value: serverTLSConfig(manager)}, nil
}

type ingressListeners struct {
	all   []net.Listener
	plain []string
	tls   []string
}

func (s *Service) openListeners(ctx context.Context, tlsConf *tls.Config) (ingressListeners, error) {
	plainAddrs := s.cfg.PlainListenAddrList()
	tlsAddrs := s.cfg.TLSListenAddrList()
	out := ingressListeners{
		all:   make([]net.Listener, 0, plainAddrs.Len()+tlsAddrs.Len()),
		plain: plainAddrs.Values(),
		tls:   tlsAddrs.Values(),
	}
	if err := s.listenPlain(ctx, plainAddrs.Values(), &out); err != nil {
		return ingressListeners{}, err
	}
	if err := s.listenTLS(ctx, tlsAddrs.Values(), tlsConf, &out); err != nil {
		return ingressListeners{}, err
	}
	if len(out.all) == 0 {
		return ingressListeners{}, oopsx.B("ingress").Errorf("no ingress listeners (configure ingress.listen and/or ingress.tls)")
	}
	return out, nil
}

func (s *Service) listenPlain(ctx context.Context, addrs []string, out *ingressListeners) error {
	for _, addr := range addrs {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		if err != nil {
			s.closeListeners(out.all)
			return oopsx.B("ingress").Wrapf(err, "listen %s", addr)
		}
		out.all = append(out.all, ln)
	}
	return nil
}

func (s *Service) listenTLS(ctx context.Context, addrs []string, tlsConf *tls.Config, out *ingressListeners) error {
	if tlsConf == nil {
		return nil
	}
	for _, addr := range addrs {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		if err != nil {
			s.closeListeners(out.all)
			return oopsx.B("ingress").Wrapf(err, "listen tls %s", addr)
		}
		out.all = append(out.all, tls.NewListener(ln, tlsConf))
	}
	return nil
}

func (s *Service) startGatewayServers(listeners []net.Listener, log *slog.Logger) error {
	snapshot, _, err := s.vale.build(nil)
	if err != nil {
		return err
	}
	gateway := s.vale.gateway(snapshot, log, true)
	s.gateway = gateway
	s.servers = makeIngressServers(len(listeners), log, gateway, s.currentRouteCount)
	serveIngressListeners(listeners, s.servers, log)
	return nil
}

func makeIngressServers(count int, log *slog.Logger, gateway *valeruntime.Gateway, routeCount func() int) []*http.Server {
	servers := make([]*http.Server, 0, count)
	for range count {
		servers = append(servers, newIngressHTTPServer(log, gateway, routeCount))
	}
	return servers
}

func serveIngressListeners(listeners []net.Listener, servers []*http.Server, log *slog.Logger) {
	for i, ln := range listeners {
		go func(ln net.Listener, server *http.Server) {
			if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Error("ingress listener stopped", "error", err)
			}
		}(ln, servers[i])
	}
}

func (s *Service) startRefreshLoop(ctx context.Context) {
	refreshCtx, refreshCancel := context.WithCancel(context.WithoutCancel(ctx))
	s.refreshCancel = refreshCancel
	ch := s.raft.DeployReconcileSignals()
	s.refreshWG.Go(func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ch:
				s.refreshRoutes()
			case <-ticker.C:
				s.refreshRoutes()
			}
		}
	})
}

func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	servers := s.servers
	s.servers = nil
	s.gateway = nil
	s.routeCount.Store(0)
	s.started = false
	cancel := s.refreshCancel
	s.refreshCancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		s.refreshWG.Wait()
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				s.logger.Warn("ingress shutdown", "error", err)
			}
		}(server)
	}
	wg.Wait()

	s.logger.Info("ingress stopped")
	return nil
}

func (s *Service) closeListeners(listeners []net.Listener) {
	for _, l := range listeners {
		if err := l.Close(); err != nil {
			s.logger.Warn("close ingress listener", "error", err)
		}
	}
}

func (s *Service) currentRouteCount() int {
	return int(s.routeCount.Load())
}
