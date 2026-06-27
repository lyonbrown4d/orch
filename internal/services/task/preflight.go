package task

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type hostPortClaim struct {
	Namespace string
	App       string
	Workload  string
	Endpoint  string
	Protocol  string
	HostIP    string
	HostPort  int
}

func (s *Service) preflightDeploy(ctx context.Context, app *deployv1.App) error {
	if err := preflightDuplicateHostPorts(*app); err != nil {
		return err
	}
	if s == nil || s.raft == nil {
		return nil
	}
	incoming := hostPortClaims(*app)
	if len(incoming) == 0 {
		return nil
	}
	if err := s.preflightExistingHostPorts(app.Metadata, incoming); err != nil {
		return err
	}
	return s.preflightLocalHostPorts(ctx, app.Metadata, incoming)
}

func (s *Service) preflightExistingHostPorts(meta deployv1.Metadata, incoming []hostPortClaim) error {
	var conflict error
	s.raft.ListDesiredDeployApps().Range(func(_ int, existing deployv1.App) bool {
		if sameAppMetadata(meta, existing.Metadata) {
			return true
		}
		next, current, ok := findHostPortConflict(incoming, hostPortClaims(existing))
		if !ok {
			return true
		}
		conflict = oopsx.B("task").Errorf("host port conflict: %s wants %s already claimed by %s", next.describe(), next.bindText(), current.describe())
		return false
	})
	return conflict
}

func (s *Service) preflightLocalHostPorts(ctx context.Context, meta deployv1.Metadata, incoming []hostPortClaim) error {
	current := sameAppHostPortClaims(s, meta)
	for _, claim := range incoming {
		if _, _, ok := findHostPortConflict([]hostPortClaim{claim}, current); ok {
			continue
		}
		if err := claim.checkLocalAvailable(ctx); err != nil {
			return err
		}
	}
	return nil
}

func sameAppHostPortClaims(s *Service, meta deployv1.Metadata) []hostPortClaim {
	if s == nil || s.raft == nil {
		return nil
	}
	app, ok := s.raft.GetDesiredDeployApp(meta)
	if !ok {
		return nil
	}
	return hostPortClaims(app)
}

func findHostPortConflict(incoming, existing []hostPortClaim) (hostPortClaim, hostPortClaim, bool) {
	for _, next := range incoming {
		for _, current := range existing {
			if hostPortClaimsConflict(next, current) {
				return next, current, true
			}
		}
	}
	return hostPortClaim{}, hostPortClaim{}, false
}

func preflightDuplicateHostPorts(app deployv1.App) error {
	claims := hostPortClaims(app)
	for i := range claims {
		for j := i + 1; j < len(claims); j++ {
			if !hostPortClaimsConflict(claims[i], claims[j]) {
				continue
			}
			return oopsx.B("task").Errorf("host port conflict: %s and %s both claim %s", claims[i].describe(), claims[j].describe(), claims[i].bindText())
		}
	}
	return nil
}

func hostPortClaims(app deployv1.App) []hostPortClaim {
	claims := make([]hostPortClaim, 0)
	for i := range app.Workloads {
		workload := &app.Workloads[i]
		for _, endpoint := range workload.Endpoints {
			if endpoint.HostPort <= 0 {
				continue
			}
			claims = append(claims, hostPortClaim{
				Namespace: workloadmeta.NamespaceOrDefault(app.Metadata.Namespace),
				App:       strings.TrimSpace(app.Metadata.Name),
				Workload:  strings.TrimSpace(workload.Name),
				Endpoint:  strings.TrimSpace(endpoint.Name),
				Protocol:  normalizeHostPortProtocol(endpoint.Protocol),
				HostIP:    normalizeHostPortIP(endpoint.HostIP),
				HostPort:  endpoint.HostPort,
			})
		}
	}
	return claims
}

func sameAppMetadata(a, b deployv1.Metadata) bool {
	return workloadmeta.NamespaceOrDefault(a.Namespace) == workloadmeta.NamespaceOrDefault(b.Namespace) && strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name)
}

func normalizeHostPortProtocol(proto deployv1.EndpointProto) string {
	if proto == deployv1.ProtoUDP {
		return string(deployv1.ProtoUDP)
	}
	return string(deployv1.ProtoTCP)
}

func normalizeHostPortIP(hostIP string) string {
	hostIP = strings.TrimSpace(hostIP)
	if hostIP == "0.0.0.0" || hostIP == "::" || hostIP == "[::]" {
		return ""
	}
	return hostIP
}

func hostPortClaimsConflict(a, b hostPortClaim) bool {
	if a.HostPort != b.HostPort || a.Protocol != b.Protocol {
		return false
	}
	return a.HostIP == "" || b.HostIP == "" || a.HostIP == b.HostIP
}

func (c hostPortClaim) checkLocalAvailable(ctx context.Context) error {
	addr := net.JoinHostPort(c.listenHost(), strconv.Itoa(c.HostPort))
	listen := &net.ListenConfig{}
	if c.Protocol == string(deployv1.ProtoUDP) {
		conn, err := listen.ListenPacket(ctx, "udp", addr)
		if err != nil {
			return oopsx.B("task").Wrapf(err, "host port unavailable: %s cannot bind %s", c.describe(), c.bindText())
		}
		if err := conn.Close(); err != nil {
			return oopsx.B("task").Wrapf(err, "close host port preflight socket")
		}
		return nil
	}
	ln, err := listen.Listen(ctx, "tcp", addr)
	if err != nil {
		return oopsx.B("task").Wrapf(err, "host port unavailable: %s cannot bind %s", c.describe(), c.bindText())
	}
	if err := ln.Close(); err != nil {
		return oopsx.B("task").Wrapf(err, "close host port preflight listener")
	}
	return nil
}

func (c hostPortClaim) listenHost() string {
	if c.HostIP == "" {
		return "0.0.0.0"
	}
	return c.HostIP
}

func (c hostPortClaim) describe() string {
	return fmt.Sprintf("%s/%s workload=%s endpoint=%s", c.Namespace, c.App, c.Workload, c.Endpoint)
}

func (c hostPortClaim) bindText() string {
	host := c.HostIP
	if host == "" {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s/%s:%d", host, c.Protocol, c.HostPort)
}
