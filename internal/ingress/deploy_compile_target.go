package ingress

import (
	"net"
	"net/url"
	"strings"

	"github.com/arcgolabs/collectionx/list"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func remoteIngressBaseURL(cluster config.ClusterConfig, ingress config.IngressConfig, nodeID string) (string, bool) {
	scheme, port, ok := ingressForwardEndpoint(ingress)
	if !ok {
		return "", false
	}
	apiURL, ok := cluster.NodeURL(nodeID)
	if !ok {
		return "", false
	}
	return ingressURLFromNodeAPI(apiURL, scheme, port)
}

func ingressForwardEndpoint(ingress config.IngressConfig) (string, string, bool) {
	if port, ok := firstListenPort(ingress.PlainListenAddrList()); ok {
		return "http", port, true
	}
	if port, ok := firstListenPort(ingress.TLSListenAddrList()); ok {
		return "https", port, true
	}
	return "", "", false
}

func firstListenPort(addrs *list.List[string]) (string, bool) {
	if addrs == nil {
		return "", false
	}
	var port string
	addrs.Range(func(_ int, addr string) bool {
		if p, ok := listenPort(addr); ok {
			port = p
			return false
		}
		return true
	})
	return port, port != ""
}

func listenPort(addr string) (string, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", false
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port, true
	}
	if port, ok := strings.CutPrefix(addr, ":"); ok {
		return port, port != ""
	}
	return "", false
}

func ingressURLFromNodeAPI(apiURL, scheme, port string) (string, bool) {
	raw := strings.TrimSpace(apiURL)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Hostname()) == "" {
		return "", false
	}
	u.Scheme = scheme
	u.Host = net.JoinHostPort(u.Hostname(), port)
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), true
}

func endpointHTTPPort(w deployv1.Workload, endpointName string) (int, bool) {
	port := 0
	found := false
	w.EndpointList().Range(func(_ int, ep deployv1.Endpoint) bool {
		if strings.TrimSpace(ep.Name) != endpointName {
			return true
		}
		if ep.Protocol != "" && ep.Protocol != deployv1.ProtoHTTP {
			return false
		}
		if ep.Port <= 0 {
			return false
		}
		port = ep.Port
		found = true
		return false
	})
	return port, found
}
