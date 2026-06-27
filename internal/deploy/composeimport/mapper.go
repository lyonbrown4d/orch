package composeimport

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"
	composetypes "github.com/compose-spec/compose-go/v2/types"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

// MapProject converts a compose-spec [composetypes.Project] into the canonical orch
// [deployv1.App]. Runtime scheduling uses only the App; this function is the compatibility edge.
func MapProject(proj *composetypes.Project) (*Result, error) {
	if proj == nil {
		return nil, errors.New("compose project is nil")
	}
	var rep Report
	app := &deployv1.App{
		APIVersion: "warden.arcgolabs.io/v1alpha1",
		Kind:       "App",
		Metadata: deployv1.Metadata{
			Name:      sanitizeProjectName(proj.Name),
			Namespace: "default",
			Annotations: map[string]string{
				"compose.arcgolabs.io/import": "true",
			},
		},
	}

	if len(proj.Networks) > 0 {
		rep.warnf("compose networks are not mapped into the canonical model yet (%d defined)", len(proj.Networks))
	}

	app.Volumes = mapComposeVolumes(proj)
	nameOrder := proj.ServiceNames()

	wloads := list.NewListWithCapacity[deployv1.Workload](len(nameOrder))
	for _, svcName := range nameOrder {
		svc := proj.Services[svcName]
		wl, extraVol, ok := mapService(svcName, svc, &rep)
		if !ok {
			continue
		}
		wloads.Add(wl)
		app.Volumes = mergeVolumesDedup(app.Volumes, extraVol)
	}
	app.Workloads = wloads.Values()

	if len(app.Workloads) == 0 {
		return nil, errors.New("compose import produced no workloads (need image per service)")
	}

	return &Result{App: app, Report: rep}, nil
}

func sanitizeProjectName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "compose-import"
	}
	out := strings.Map(sanitizeProjectRune, n)
	if out == "" || out[0] < 'A' || (out[0] > 'Z' && out[0] < 'a') {
		return "compose-" + out
	}
	return out
}

func sanitizeProjectRune(r rune) rune {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return r
	case r == '_' || r == '-' || r == '.':
		return r
	default:
		return '-'
	}
}

func mapComposeVolumes(proj *composetypes.Project) []deployv1.Volume {
	if proj.Volumes == nil {
		return nil
	}
	return list.MapList(sortedMapKeys(proj.Volumes), func(_ int, name string) deployv1.Volume {
		return deployv1.Volume{Name: name, Persistent: true}
	}).Values()
}

func mergeVolumesDedup(existing, add []deployv1.Volume) []deployv1.Volume {
	seen := set.NewSetWithCapacity[string](len(existing) + len(add))
	for _, v := range existing {
		seen.Add(v.Name)
	}
	out := existing
	for _, v := range add {
		if seen.Contains(v.Name) {
			continue
		}
		seen.Add(v.Name)
		out = append(out, v)
	}
	return out
}

func mapService(name string, s composetypes.ServiceConfig, rep *Report) (deployv1.Workload, []deployv1.Volume, bool) {
	image := strings.TrimSpace(s.Image)
	if image == "" {
		rep.warnf("service %q: skipped (no image; compose build-only services are not mapped yet)", name)
		return deployv1.Workload{}, nil, false
	}

	w := deployv1.Workload{
		Name:    name,
		Kind:    deployv1.WorkloadKindService,
		Runtime: deployv1.RuntimeDocker,
		Run: deployv1.RunSpec{
			Artifact: deployv1.ArtifactSpec{Image: image},
			Exec: deployv1.ExecSpec{
				Command: []string(s.Entrypoint),
				Args:    []string(s.Command),
			},
			Env: envFromCompose(s.Environment),
			Cwd: strings.TrimSpace(s.WorkingDir),
		},
		Replicas: composeReplicas(&s),
	}

	w.Run.Options.Docker = dockerOptionsFromCompose(name, &s, rep)

	w.Endpoints = endpointsFromCompose(name, s.Ports, rep)
	w.DependsOn = dependsFromCompose(s.DependsOn, rep)

	if res := resourcesFromCompose(s.Deploy); res != nil {
		w.Resources = res
	}

	if s.HealthCheck != nil && !s.HealthCheck.Disable {
		rep.warnf("service %q: healthcheck is not mapped to canonical HTTP/TCP probes yet", name)
	}

	var extraVol []deployv1.Volume
	if s.Volumes != nil {
		w.Mounts, extraVol = mountsFromCompose(name, s.Volumes, rep)
	}

	return w, extraVol, true
}

func envFromCompose(m composetypes.MappingWithEquals) []deployv1.EnvVar {
	if len(m) == 0 {
		return nil
	}
	flat := m.ToMapping()
	return list.MapList(sortedMapKeys(flat), func(_ int, k string) deployv1.EnvVar {
		return deployv1.EnvVar{Name: k, Value: flat[k]}
	}).Values()
}

func composeReplicas(s *composetypes.ServiceConfig) int {
	replicas := s.GetScale()
	if replicas <= 0 {
		return 1
	}
	return replicas
}

func endpointsFromCompose(service string, ports []composetypes.ServicePortConfig, rep *Report) []deployv1.Endpoint {
	if len(ports) == 0 {
		return nil
	}
	var out []deployv1.Endpoint
	for i, port := range ports {
		endpoint, ok := endpointFromComposePort(service, i, port, rep)
		if ok {
			out = append(out, endpoint)
		}
	}
	return out
}

func endpointFromComposePort(service string, index int, port composetypes.ServicePortConfig, rep *Report) (deployv1.Endpoint, bool) {
	if port.Target == 0 {
		rep.warnf("service %q: ports[%d] has no container target port, skipped", service, index)
		return deployv1.Endpoint{}, false
	}
	proto := endpointProtoFromCompose(service, port, rep)
	endpoint := deployv1.Endpoint{
		Name:     endpointNameFromComposePort(proto, port.Target),
		Port:     int(port.Target),
		Protocol: proto,
		HostIP:   strings.TrimSpace(port.HostIP),
	}
	applyEndpointPublishedPort(service, &endpoint, port, rep)
	return endpoint, true
}

func endpointProtoFromCompose(service string, port composetypes.ServicePortConfig, rep *Report) deployv1.EndpointProto {
	switch strings.ToLower(port.Protocol) {
	case "udp":
		return deployv1.ProtoUDP
	case "tcp", "":
		return deployv1.ProtoTCP
	default:
		rep.warnf("service %q: port %d protocol %q mapped as tcp", service, port.Target, port.Protocol)
		return deployv1.ProtoTCP
	}
}

func endpointNameFromComposePort(proto deployv1.EndpointProto, target uint32) string {
	protoStr := strings.ToLower(string(proto))
	if protoStr == "" {
		protoStr = "tcp"
	}
	return fmt.Sprintf("%s-%d", protoStr, target)
}

func applyEndpointPublishedPort(service string, endpoint *deployv1.Endpoint, port composetypes.ServicePortConfig, rep *Report) {
	published := strings.TrimSpace(port.Published)
	if endpoint == nil || published == "" {
		return
	}
	hostPort, err := strconv.Atoi(published)
	if err != nil {
		rep.warnf("service %q: port %d published value %q is not a single host port", service, port.Target, published)
		return
	}
	endpoint.HostPort = hostPort
}
func dependsFromCompose(d composetypes.DependsOnConfig, rep *Report) []deployv1.WorkloadRef {
	if len(d) == 0 {
		return nil
	}
	names := sortedMapKeys(d)
	out := list.NewListWithCapacity[deployv1.WorkloadRef](names.Len())
	names.Range(func(_ int, name string) bool {
		dep := d[name]
		switch strings.TrimSpace(dep.Condition) {
		case "", composetypes.ServiceConditionStarted, composetypes.ServiceConditionHealthy:
			out.Add(deployv1.WorkloadRef{Name: name})
		case composetypes.ServiceConditionCompletedSuccessfully:
			rep.warnf("depends_on service %q uses condition %q (not modeled); treating as dependency edge only", name, dep.Condition)
			out.Add(deployv1.WorkloadRef{Name: name})
		default:
			rep.warnf("depends_on service %q uses condition %q; treating as ordered dependency", name, dep.Condition)
			out.Add(deployv1.WorkloadRef{Name: name})
		}
		return true
	})
	return out.Values()
}

func sortedMapKeys[V any](m map[string]V) *list.List[string] {
	keys := list.NewList(mapping.NewMapFrom(m).Keys()...)
	keys.Sort(strings.Compare)
	return keys
}
