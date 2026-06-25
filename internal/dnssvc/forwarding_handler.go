package dnssvc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"
)

const workloadDNSForwardTimeout = 2 * time.Second

type workloadIPv4Lookup interface {
	LookupWorkloadIPv4(namespace, workloadName string) (string, bool)
}

type forwardingHandler struct {
	resolver  *dnsserver.Resolver
	upstreams *list.List[string]
	logger    *slog.Logger
	zone      string
	workloads workloadIPv4Lookup
}

// NewForwardingHandler builds the workload DNS forwarding handler.
func NewForwardingHandler(resolver *dnsserver.Resolver, upstreams *list.List[string], logger *slog.Logger) dns.Handler {
	return newForwardingHandler(resolver, upstreams, logger, "orch.local", nil)
}

func newForwardingHandler(resolver *dnsserver.Resolver, upstreams *list.List[string], logger *slog.Logger, zone string, workloads workloadIPv4Lookup) dns.Handler {
	if upstreams == nil {
		upstreams = list.NewList[string]()
	}
	if logger == nil {
		logger = slog.Default()
	}
	zone = strings.Trim(strings.ToLower(strings.TrimSpace(zone)), ".")
	if zone == "" {
		zone = "orch.local"
	}
	return &forwardingHandler{
		resolver:  resolver,
		upstreams: upstreams,
		logger:    logger,
		zone:      zone,
		workloads: workloads,
	}
}

func (h *forwardingHandler) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.Authoritative = false
	reply.RecursionAvailable = h.upstreams.Len() > 0

	if request.Opcode != dns.OpcodeQuery {
		reply.Rcode = dns.RcodeNotImplemented
		writeDNSReply(h.logger, writer, reply)
		return
	}
	if len(request.Question) == 0 {
		reply.Rcode = dns.RcodeFormatError
		writeDNSReply(h.logger, writer, reply)
		return
	}
	if h.resolver == nil {
		reply.Rcode = dns.RcodeRefused
		writeDNSReply(h.logger, writer, reply)
		return
	}

	resolution, err := h.resolver.Resolve(context.Background(), request.Question[0])
	if err != nil {
		h.logger.Error("dns resolve failed", "error", err, "name", request.Question[0].Name, "type", request.Question[0].Qtype)
		reply.Rcode = dns.RcodeServerFailure
		writeDNSReply(h.logger, writer, reply)
		return
	}
	if h.writeDynamicWorkloadA(writer, request, reply, resolution.RCode) {
		return
	}
	if resolution.RCode == dns.RcodeRefused && h.upstreams.Len() > 0 {
		h.forward(writer, request)
		return
	}

	reply.Rcode = resolution.RCode
	reply.Authoritative = resolution.Authoritative
	reply.Answer = resolution.Answer
	reply.Ns = resolution.Authority
	reply.Extra = resolution.Extra
	writeDNSReply(h.logger, writer, reply)
}

func (h *forwardingHandler) writeDynamicWorkloadA(writer dns.ResponseWriter, request *dns.Msg, reply *dns.Msg, rcode int) bool {
	if h.workloads == nil || (rcode != dns.RcodeNameError && rcode != dns.RcodeRefused) {
		return false
	}
	question := request.Question[0]
	if question.Qtype != dns.TypeA && question.Qtype != dns.TypeANY {
		return false
	}
	namespace, workload, ok := parseWorkloadServiceFQDN(question.Name, h.zone)
	if !ok {
		return false
	}
	address, ok := h.workloads.LookupWorkloadIPv4(namespace, workload)
	if !ok {
		return false
	}
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return false
	}
	reply.Rcode = dns.RcodeSuccess
	reply.Authoritative = true
	reply.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(question.Name),
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: ip,
	}}
	writeDNSReply(h.logger, writer, reply)
	return true
}

func (h *forwardingHandler) forward(writer dns.ResponseWriter, request *dns.Msg) {
	response, lastErr := h.firstUpstreamResponse(request)
	if response != nil {
		response.RecursionAvailable = true
		writeDNSReply(h.logger, writer, response)
		return
	}
	if lastErr != nil {
		h.logger.Warn("dns upstream forward failed", "error", lastErr, "name", request.Question[0].Name)
	}

	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.RecursionAvailable = true
	reply.Rcode = dns.RcodeServerFailure
	writeDNSReply(h.logger, writer, reply)
}

func (h *forwardingHandler) firstUpstreamResponse(request *dns.Msg) (*dns.Msg, error) {
	var lastErr error
	var response *dns.Msg
	h.upstreams.Range(func(_ int, upstream string) bool {
		ctx, cancel := context.WithTimeout(context.Background(), workloadDNSForwardTimeout)
		defer cancel()
		got, err := exchangeUpstreamDNS(ctx, request, upstream)
		if got != nil && err == nil {
			response = got
			return false
		}
		lastErr = err
		return true
	})
	return response, lastErr
}

func exchangeUpstreamDNS(ctx context.Context, request *dns.Msg, upstream string) (*dns.Msg, error) {
	query := request.Copy()
	query.RecursionDesired = true
	response, err := exchangeUpstreamDNSNet(ctx, "udp", query, upstream)
	if err != nil || response == nil || !response.Truncated {
		return response, err
	}
	return exchangeUpstreamDNSNet(ctx, "tcp", query, upstream)
}

func exchangeUpstreamDNSNet(ctx context.Context, network string, query *dns.Msg, upstream string) (*dns.Msg, error) {
	response, _, err := (&dns.Client{Net: network, Timeout: workloadDNSForwardTimeout}).ExchangeContext(ctx, query, upstream)
	if err != nil {
		return response, fmt.Errorf("exchange upstream dns over %s: %w", network, err)
	}
	return response, nil
}

func writeDNSReply(logger *slog.Logger, writer dns.ResponseWriter, msg *dns.Msg) {
	if err := writer.WriteMsg(msg); err != nil && logger != nil {
		logger.Warn("write dns response", "error", err)
	}
}

var _ dns.Handler = (*forwardingHandler)(nil)
