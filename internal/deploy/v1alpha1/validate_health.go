package v1alpha1

import (
	"strings"
	"time"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (w *Workload) validateHealth() error {
	if w.Health == nil {
		return nil
	}
	checks := []struct {
		name  string
		probe *Probe
	}{
		{name: "readiness", probe: w.Health.Readiness},
		{name: "liveness", probe: w.Health.Liveness},
		{name: "startup", probe: w.Health.Startup},
	}
	for _, check := range checks {
		if err := w.validateProbe(check.name, check.probe); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workload) validateProbe(name string, probe *Probe) error {
	if probe == nil {
		return nil
	}
	if err := validateProbeTiming(name, probe); err != nil {
		return err
	}
	return w.validateProbeKind(name, probe)
}

func validateProbeTiming(name string, probe *Probe) error {
	if probe.Retries < 0 {
		return oopsx.B("deploy").Errorf("health.%s.retries must be >= 0", name)
	}
	if err := validateProbeDuration("health."+name+".interval", probe.Interval); err != nil {
		return err
	}
	if err := validateProbeDuration("health."+name+".timeout", probe.Timeout); err != nil {
		return err
	}
	return validateProbeDuration("health."+name+".startPeriod", probe.StartPeriod)
}

func (w *Workload) validateProbeKind(name string, probe *Probe) error {
	kinds := probeKindCount(probe)
	if kinds == 0 {
		return oopsx.B("deploy").Errorf("health.%s probe kind is required", name)
	}
	if kinds > 1 {
		return oopsx.B("deploy").Errorf("health.%s must specify exactly one probe kind", name)
	}
	if probe.HTTP != nil {
		return w.validateHTTPProbe(name, probe.HTTP)
	}
	return w.validateTCPProbe(name, probe.TCP)
}

func probeKindCount(probe *Probe) int {
	kinds := 0
	if probe.HTTP != nil {
		kinds++
	}
	if probe.TCP != nil {
		kinds++
	}
	return kinds
}

func validateProbeDuration(field, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if _, err := time.ParseDuration(raw); err != nil {
		return oopsx.B("deploy").Wrapf(err, "%s is invalid", field)
	}
	return nil
}

func (w *Workload) validateHTTPProbe(name string, probe *HTTPProbe) error {
	if err := validateProbePort("health."+name+".http.port", probe.Port); err != nil {
		return err
	}
	return w.validateProbeEndpoint("health."+name+".http.endpoint", probe.Endpoint, ProtoHTTP, probe.Port)
}

func (w *Workload) validateTCPProbe(name string, probe *TCPProbe) error {
	if err := validateProbePort("health."+name+".tcp.port", probe.Port); err != nil {
		return err
	}
	return w.validateProbeEndpoint("health."+name+".tcp.endpoint", probe.Endpoint, ProtoTCP, probe.Port)
}

func validateProbePort(field string, port int) error {
	if port == 0 {
		return nil
	}
	if port < 0 || port > 65535 {
		return oopsx.B("deploy").Errorf("%s must be 1..65535 (got %d)", field, port)
	}
	return nil
}

func (w *Workload) validateProbeEndpoint(field string, ref EndpointRef, want EndpointProto, explicitPort int) error {
	if workload := strings.TrimSpace(ref.Workload); workload != "" && workload != w.Name {
		return oopsx.B("deploy").Errorf("%s.workload must be empty or %q", field, w.Name)
	}
	endpointName := strings.TrimSpace(ref.Endpoint)
	if endpointName == "" {
		return w.validateDefaultProbeEndpoint(field, want, explicitPort)
	}
	endpoint, ok := w.endpointByName(endpointName)
	if !ok {
		return oopsx.B("deploy").Errorf("%s references unknown endpoint %q", field, endpointName)
	}
	if !probeEndpointProtocolMatches(endpoint.Protocol, want) {
		return oopsx.B("deploy").Errorf("%s references %s endpoint %q", field, endpoint.Protocol, endpointName)
	}
	return nil
}

func (w *Workload) validateDefaultProbeEndpoint(field string, want EndpointProto, explicitPort int) error {
	if explicitPort > 0 || w.hasProbeEndpoint(want) {
		return nil
	}
	return oopsx.B("deploy").Errorf("%s or port is required", field)
}

func (w *Workload) hasProbeEndpoint(want EndpointProto) bool {
	_, ok := w.firstProbeEndpoint(want)
	return ok
}

func (w *Workload) firstProbeEndpoint(want EndpointProto) (Endpoint, bool) {
	var found Endpoint
	ok := false
	w.EndpointList().Range(func(_ int, endpoint Endpoint) bool {
		if probeEndpointProtocolMatches(endpoint.Protocol, want) {
			found = endpoint
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func (w *Workload) endpointByName(name string) (Endpoint, bool) {
	var found Endpoint
	ok := false
	w.EndpointList().Range(func(_ int, endpoint Endpoint) bool {
		if strings.TrimSpace(endpoint.Name) == name {
			found = endpoint
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func probeEndpointProtocolMatches(actual, want EndpointProto) bool {
	if want == ProtoTCP {
		return actual == ProtoTCP || actual == ProtoHTTP
	}
	return actual == want
}
