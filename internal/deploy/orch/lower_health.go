package orch

import (
	"errors"
	"strings"

	"github.com/arcgolabs/mapper"
	"github.com/arcgolabs/plano/compiler"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func fillWorkloadHealth(m *mapper.Mapper, workload *v1.Workload, f *compiler.HIRForm) error {
	blocks := childFormsByKind(f, "health")
	if len(blocks) > 1 {
		return errors.New("at most one health block")
	}
	if len(blocks) == 0 {
		return nil
	}
	health, err := lowerHealth(m, &blocks[0])
	if err != nil {
		return err
	}
	workload.Health = health
	return nil
}

func lowerHealth(_ *mapper.Mapper, f *compiler.HIRForm) (*v1.Health, error) {
	health := &v1.Health{}
	if probe, ok := lowerProbeFromFields(f); ok {
		health.Readiness = probe
	}
	if err := lowerHealthNamedProbes(f, health); err != nil {
		return nil, err
	}
	return health, nil
}

func lowerHealthNamedProbes(f *compiler.HIRForm, health *v1.Health) error {
	readiness, ok, err := lowerNamedProbe(f, "readiness")
	if err != nil {
		return err
	}
	if ok {
		health.Readiness = readiness
	}
	liveness, ok, err := lowerNamedProbe(f, "liveness")
	if err != nil {
		return err
	}
	if ok {
		health.Liveness = liveness
	}
	startup, ok, err := lowerNamedProbe(f, "startup")
	if err != nil {
		return err
	}
	if ok {
		health.Startup = startup
	}
	return nil
}

func lowerNamedProbe(f *compiler.HIRForm, name string) (*v1.Probe, bool, error) {
	blocks := childFormsByKind(f, name)
	if len(blocks) > 1 {
		return nil, false, errors.New("at most one " + name + " block")
	}
	if len(blocks) == 0 {
		return nil, false, nil
	}
	probe, _ := lowerProbeFromFields(&blocks[0])
	return probe, true, nil
}

func lowerProbeFromFields(f *compiler.HIRForm) (*v1.Probe, bool) {
	if !hasProbeFields(f) {
		return nil, false
	}
	probe := lowerProbeTiming(f)
	attachProbeTransport(probe, f)
	return probe, true
}

func lowerProbeTiming(f *compiler.HIRForm) *v1.Probe {
	probe := &v1.Probe{}
	if interval, ok := stringField(f, "interval"); ok {
		probe.Interval = interval
	}
	if timeout, ok := stringField(f, "timeout"); ok {
		probe.Timeout = timeout
	}
	if retries, ok := intField(f, "retries"); ok {
		probe.Retries = retries
	}
	if startPeriod, ok := stringField(f, "start_period"); ok {
		probe.StartPeriod = startPeriod
	}
	return probe
}

func attachProbeTransport(probe *v1.Probe, f *compiler.HIRForm) {
	httpPath, hasHTTP := probeHTTPPath(f)
	endpoint := probeEndpointRef(f)
	port, hasPort := intField(f, "port")
	if shouldAttachHTTPProbe(f, endpoint, hasHTTP, hasPort) {
		probe.HTTP = &v1.HTTPProbe{Path: httpPath, Endpoint: endpoint}
		if hasPort {
			probe.HTTP.Port = port
		}
	}
	if probeTCPEnabled(f) {
		probe.TCP = &v1.TCPProbe{Endpoint: endpoint}
		if hasPort {
			probe.TCP.Port = port
		}
	}
}

func shouldAttachHTTPProbe(f *compiler.HIRForm, endpoint v1.EndpointRef, hasHTTP, hasPort bool) bool {
	return hasHTTP || (!probeTCPEnabled(f) && (endpoint.Endpoint != "" || hasPort))
}

func hasProbeFields(f *compiler.HIRForm) bool {
	if hasProbeStringField(f) {
		return true
	}
	if enabled, ok := boolField(f, "tcp"); ok && enabled {
		return true
	}
	if port, ok := intField(f, "port"); ok && port != 0 {
		return true
	}
	if retries, ok := intField(f, "retries"); ok && retries != 0 {
		return true
	}
	return false
}

func hasProbeStringField(f *compiler.HIRForm) bool {
	checks := []string{"http", "path", "endpoint", "interval", "timeout", "start_period"}
	for _, name := range checks {
		if value, ok := stringField(f, name); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func probeHTTPPath(f *compiler.HIRForm) (string, bool) {
	if path, ok := stringField(f, "http"); ok && strings.TrimSpace(path) != "" {
		return path, true
	}
	if path, ok := stringField(f, "path"); ok && strings.TrimSpace(path) != "" {
		return path, true
	}
	return "", false
}

func probeEndpointRef(f *compiler.HIRForm) v1.EndpointRef {
	endpoint, ok := stringField(f, "endpoint")
	if !ok || strings.TrimSpace(endpoint) == "" {
		return v1.EndpointRef{}
	}
	return v1.EndpointRef{Endpoint: strings.TrimSpace(endpoint)}
}

func probeTCPEnabled(f *compiler.HIRForm) bool {
	enabled, ok := boolField(f, "tcp")
	return ok && enabled
}
