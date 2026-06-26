package task

import (
	"context"
	"fmt"
	"net"
	"net/http"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const (
	defaultReadinessInterval = time.Second
	defaultReadinessTimeout  = 2 * time.Second
	defaultReadinessAttempts = 30
	defaultHTTPProbePath     = "/"
	defaultProbeHost         = "127.0.0.1"
)

type readinessCheck struct {
	kind        string
	host        string
	port        int
	path        string
	interval    time.Duration
	timeout     time.Duration
	attempts    int
	startPeriod time.Duration
}

func (s *Service) waitWorkloadReadiness(ctx context.Context, meta deployv1.Metadata, workload deployv1.Workload, address string) error {
	probe := workloadReadinessProbe(workload)
	if probe == nil {
		return nil
	}
	check, err := s.buildReadinessCheck(meta, workload, *probe, address)
	if err != nil {
		return err
	}
	if err := sleepContext(ctx, check.startPeriod); err != nil {
		return err
	}
	return s.waitReadinessAttempts(ctx, workload, check)
}

func (s *Service) waitReadinessAttempts(ctx context.Context, workload deployv1.Workload, check readinessCheck) error {
	var lastErr error
	for attempt := 1; attempt <= check.attempts; attempt++ {
		lastErr = runReadinessCheck(ctx, check)
		if lastErr == nil {
			s.logger.Info("workload readiness probe succeeded", "workload", workload.Name, "kind", check.kind, "attempt", attempt)
			return nil
		}
		if err := waitBeforeNextReadinessAttempt(ctx, check, attempt); err != nil {
			return err
		}
	}
	return oopsx.B("task", "health").Wrapf(lastErr, "workload %s readiness probe failed after %d attempt(s)", workload.Name, check.attempts)
}

func waitBeforeNextReadinessAttempt(ctx context.Context, check readinessCheck, attempt int) error {
	if attempt >= check.attempts {
		return nil
	}
	return sleepContext(ctx, check.interval)
}

func workloadReadinessProbe(workload deployv1.Workload) *deployv1.Probe {
	if workload.Health == nil {
		return nil
	}
	return workload.Health.Readiness
}

func (s *Service) buildReadinessCheck(meta deployv1.Metadata, workload deployv1.Workload, probe deployv1.Probe, address string) (readinessCheck, error) {
	host, embeddedPort := splitProbeAddress(address)
	check := readinessCheck{
		host:        host,
		interval:    probeDurationOrDefault(probe.Interval, defaultReadinessInterval),
		timeout:     probeDurationOrDefault(probe.Timeout, defaultReadinessTimeout),
		attempts:    probeAttemptsOrDefault(probe.Retries),
		startPeriod: probeDurationOrDefault(probe.StartPeriod, 0),
	}
	if probe.HTTP != nil {
		return readinessHTTPCheck(meta, workload, probe.HTTP, check, embeddedPort)
	}
	if probe.TCP != nil {
		return readinessTCPCheck(meta, workload, probe.TCP, check, embeddedPort)
	}
	return readinessCheck{}, oopsx.B("task", "health").Errorf("workload %s readiness probe kind is required", workload.Name)
}

func readinessHTTPCheck(meta deployv1.Metadata, workload deployv1.Workload, probe *deployv1.HTTPProbe, check readinessCheck, embeddedPort int) (readinessCheck, error) {
	port, err := probePort(workload, probe.Endpoint.Endpoint, deployv1.ProtoHTTP, probe.Port, embeddedPort)
	if err != nil {
		return readinessCheck{}, oopsx.B("task", "health").Wrapf(err, "resolve readiness http target for %s/%s", meta.Name, workload.Name)
	}
	check.kind = "http"
	check.port = port
	check.path = normalizeHTTPProbePath(probe.Path)
	return check, nil
}

func readinessTCPCheck(meta deployv1.Metadata, workload deployv1.Workload, probe *deployv1.TCPProbe, check readinessCheck, embeddedPort int) (readinessCheck, error) {
	port, err := probePort(workload, probe.Endpoint.Endpoint, deployv1.ProtoTCP, probe.Port, embeddedPort)
	if err != nil {
		return readinessCheck{}, oopsx.B("task", "health").Wrapf(err, "resolve readiness tcp target for %s/%s", meta.Name, workload.Name)
	}
	check.kind = "tcp"
	check.port = port
	return check, nil
}

func probeDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func probeAttemptsOrDefault(retries int) int {
	if retries <= 0 {
		return defaultReadinessAttempts
	}
	return retries
}

func splitProbeAddress(address string) (string, int) {
	address = strings.TrimSpace(address)
	if address == "" {
		return defaultProbeHost, 0
	}
	if parsed, err := urlpkg.Parse(address); err == nil && parsed.Host != "" {
		address = parsed.Host
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return address, 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return host, 0
	}
	return host, port
}

func probePort(workload deployv1.Workload, endpointName string, want deployv1.EndpointProto, explicitPort, fallbackPort int) (int, error) {
	if explicitPort > 0 {
		return explicitPort, nil
	}
	endpointName = strings.TrimSpace(endpointName)
	if endpointName != "" {
		return namedProbePort(workload, endpointName, want)
	}
	if fallbackPort > 0 {
		return fallbackPort, nil
	}
	endpoint, ok := firstProbeEndpoint(workload, want)
	if !ok {
		return 0, fmt.Errorf("no %s endpoint or explicit port", want)
	}
	return endpoint.Port, nil
}

func namedProbePort(workload deployv1.Workload, endpointName string, want deployv1.EndpointProto) (int, error) {
	endpoint, ok := workloadEndpointByName(workload, endpointName)
	if !ok {
		return 0, fmt.Errorf("unknown endpoint %q", endpointName)
	}
	if !probeEndpointMatches(endpoint.Protocol, want) {
		return 0, fmt.Errorf("endpoint %q uses %s", endpointName, endpoint.Protocol)
	}
	return endpoint.Port, nil
}

func workloadEndpointByName(workload deployv1.Workload, name string) (deployv1.Endpoint, bool) {
	var found deployv1.Endpoint
	ok := false
	workload.EndpointList().Range(func(_ int, endpoint deployv1.Endpoint) bool {
		if strings.TrimSpace(endpoint.Name) == name {
			found = endpoint
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func firstProbeEndpoint(workload deployv1.Workload, want deployv1.EndpointProto) (deployv1.Endpoint, bool) {
	var found deployv1.Endpoint
	ok := false
	workload.EndpointList().Range(func(_ int, endpoint deployv1.Endpoint) bool {
		if probeEndpointMatches(endpoint.Protocol, want) {
			found = endpoint
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func probeEndpointMatches(actual, want deployv1.EndpointProto) bool {
	if want == deployv1.ProtoTCP {
		return actual == deployv1.ProtoTCP || actual == deployv1.ProtoHTTP
	}
	return actual == want
}

func normalizeHTTPProbePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultHTTPProbePath
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func runReadinessCheck(ctx context.Context, check readinessCheck) error {
	switch check.kind {
	case "http":
		return runHTTPReadinessCheck(ctx, check)
	case "tcp":
		return runTCPReadinessCheck(ctx, check)
	default:
		return fmt.Errorf("unsupported readiness probe kind %q", check.kind)
	}
}

func runHTTPReadinessCheck(ctx context.Context, check readinessCheck) error {
	client := http.Client{Timeout: check.timeout}
	targetURL := "http://" + net.JoinHostPort(check.host, strconv.Itoa(check.port)) + check.path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create http readiness request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute http readiness request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}
	return fmt.Errorf("http readiness returned status %d", resp.StatusCode)
}

func runTCPReadinessCheck(ctx context.Context, check readinessCheck) error {
	dialer := net.Dialer{Timeout: check.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(check.host, strconv.Itoa(check.port)))
	if err != nil {
		return fmt.Errorf("dial tcp readiness target: %w", err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close tcp readiness connection: %w", err)
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("readiness wait canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
