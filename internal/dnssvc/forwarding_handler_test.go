package dnssvc_test

import (
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func TestServiceForwardsNonOrchQueriesToWorkloadUpstream(t *testing.T) {
	t.Parallel()

	var upstreamQueries atomic.Int32
	upstream := startTestDNSServer(t, func(writer dns.ResponseWriter, request *dns.Msg) {
		upstreamQueries.Add(1)
		reply := new(dns.Msg)
		reply.SetReply(request)
		reply.RecursionAvailable = true
		reply.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{
				Name:   request.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    30,
			},
			A: net.ParseIP("203.0.113.10"),
		}}
		if err := writer.WriteMsg(reply); err != nil {
			t.Errorf("write upstream dns reply: %v", err)
		}
	})

	dnsCfg := config.DNSConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Zone:    "orch.local",
	}
	dnsCfg.Data.Path = filepath.Join(t.TempDir(), "dns.db")
	dnsCfg.Workload.Upstream = []string{upstream}
	svc := dnssvc.New(config.Config{DNS: dnsCfg}, testDNSLogger())

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start service: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.Stop(ctx); err != nil {
			t.Fatalf("stop service: %v", err)
		}
	})

	response := queryTestDNS(t, svc.UDPAddr(), "www.example.net.", dns.TypeA)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("external response rcode=%d answer=%#v", response.Rcode, response.Answer)
	}
	if upstreamQueries.Load() != 1 {
		t.Fatalf("upstream queries = %d, want 1", upstreamQueries.Load())
	}

	response = queryTestDNS(t, svc.UDPAddr(), "missing.orch.local.", dns.TypeA)
	if response.Rcode != dns.RcodeNameError {
		t.Fatalf("orch-zone miss rcode = %d, want NXDOMAIN", response.Rcode)
	}
	if upstreamQueries.Load() != 1 {
		t.Fatalf("orch-zone query was forwarded, upstream queries = %d", upstreamQueries.Load())
	}
}

type fakeAssignmentLookup struct {
	items *list.List[workloadmeta.Assignment]
}

func (f fakeAssignmentLookup) ListWorkloadAssignments() *list.List[workloadmeta.Assignment] {
	if f.items == nil {
		return list.NewList[workloadmeta.Assignment]()
	}
	return f.items
}

func TestServiceAnswersWorkloadDNSFromReplicatedAssignment(t *testing.T) {
	t.Parallel()

	dnsCfg := config.DNSConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Zone:    "orch.local",
	}
	dnsCfg.Data.Path = filepath.Join(t.TempDir(), "dns.db")
	assignments := fakeAssignmentLookup{items: list.NewList(workloadmeta.Assignment{
		Metadata: deployv1.Metadata{Name: "demo", Namespace: "default"},
		Workload: "remote",
		Status:   workloadmeta.AssignmentStatusRunning,
		Address:  "172.31.241.22",
	})}
	svc := dnssvc.NewWithAssignmentLookup(config.Config{DNS: dnsCfg}, testDNSLogger(), assignments)

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start service: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.Stop(ctx); err != nil {
			t.Fatalf("stop service: %v", err)
		}
	})

	response := queryTestDNS(t, svc.UDPAddr(), "remote.default.svc.orch.local.", dns.TypeA)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("assignment response rcode=%d answer=%#v", response.Rcode, response.Answer)
	}
	a, ok := response.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.ParseIP("172.31.241.22")) {
		t.Fatalf("assignment answer = %#v", response.Answer[0])
	}
}
func startTestDNSServer(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()

	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	server := &dns.Server{
		PacketConn: conn,
		Handler:    handler,
	}
	go func() {
		if err := server.ActivateAndServe(); err != nil {
			t.Logf("dns test server stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Fatalf("shutdown dns test server: %v", err)
		}
	})
	return conn.LocalAddr().String()
}

func queryTestDNS(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()

	msg := new(dns.Msg)
	msg.SetQuestion(name, qtype)
	response, _, err := (&dns.Client{Net: "udp"}).Exchange(msg, addr)
	if err != nil {
		t.Fatalf("dns query %s @ %s: %v", name, addr, err)
	}
	if response == nil {
		t.Fatalf("dns query %s @ %s returned nil response", name, addr)
	}
	return response
}

func TestForwardingHandlerWithoutUpstreamRefusesExternalQueries(t *testing.T) {
	t.Parallel()

	store, err := dnsserver.OpenBboltStore(filepath.Join(t.TempDir(), "dns.db"), testDNSLogger())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close dns store: %v", err)
		}
	})
	if err := store.SaveRecord(context.Background(), dnsserver.Record{
		Zone: "orch.local",
		Name: "orch.local",
		TTL:  60,
		Type: dns.TypeA,
		Data: "127.0.0.1",
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	resolver := dnsserver.NewResolver(store, dnsserver.WithResolverLogger(testDNSLogger()))
	server := startTestDNSServer(t, dnssvc.NewForwardingHandler(resolver, nil, nil).ServeDNS)
	response := queryTestDNS(t, server, "www.example.net.", dns.TypeA)
	if response.Rcode != dns.RcodeRefused {
		t.Fatalf("external rcode = %d, want refused", response.Rcode)
	}
}

func testDNSLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
