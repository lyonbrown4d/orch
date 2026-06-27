package orch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/arcgolabs/plano/compiler"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func lowerEndpoint(f *compiler.HIRForm) (v1.Endpoint, error) {
	var endpoint v1.Endpoint
	name, err := symbolLabelName(f)
	if err != nil {
		return endpoint, fmt.Errorf("endpoint: %w", err)
	}
	endpoint.Name = name
	port, ok := intField(f, "port")
	if !ok {
		return endpoint, fmt.Errorf("endpoint %q: port is required", name)
	}
	endpoint.Port = port
	proto, ok := stringField(f, "protocol")
	if !ok {
		return endpoint, fmt.Errorf("endpoint %q: protocol is required", name)
	}
	endpoint.Protocol = v1.EndpointProto(strings.ToLower(strings.TrimSpace(proto)))
	applyEndpointPublishFields(&endpoint, f)
	return endpoint, nil
}

func applyEndpointPublishFields(endpoint *v1.Endpoint, f *compiler.HIRForm) {
	if endpoint == nil || f == nil {
		return
	}
	if hostPort, ok := intField(f, "host_port"); ok {
		endpoint.HostPort = hostPort
	}
	if hostIP, ok := stringField(f, "host_ip"); ok {
		endpoint.HostIP = strings.TrimSpace(hostIP)
	}
}

func endpointPublishFromCall(endpoint *v1.Endpoint, call compiler.HIRCall, hostPortArg, hostIPArg int) {
	if endpoint == nil {
		return
	}
	if hostPort, ok := callIntArg(call, hostPortArg); ok {
		endpoint.HostPort = hostPort
	}
	if hostIP, ok := callStringArg(call, hostIPArg); ok {
		endpoint.HostIP = strings.TrimSpace(hostIP)
	}
}

func lowerEndpointCalls(f *compiler.HIRForm) []v1.Endpoint {
	if f == nil {
		return nil
	}
	var out []v1.Endpoint
	for i := range f.Calls.Len() {
		call, _ := f.Calls.Get(i)
		endpoint, ok := endpointFromCall(call)
		if ok {
			out = append(out, endpoint)
		}
	}
	return out
}

func endpointFromCall(call compiler.HIRCall) (v1.Endpoint, bool) {
	switch call.Name {
	case "http":
		return endpointFromProtocolCall(call, v1.ProtoHTTP, "http")
	case "tcp":
		return endpointFromProtocolCall(call, v1.ProtoTCP, "")
	case "udp":
		return endpointFromProtocolCall(call, v1.ProtoUDP, "")
	case "port":
		return endpointFromPortCall(call)
	default:
		return v1.Endpoint{}, false
	}
}

func endpointFromProtocolCall(call compiler.HIRCall, proto v1.EndpointProto, defaultName string) (v1.Endpoint, bool) {
	port, ok := callIntArg(call, 0)
	if !ok {
		return v1.Endpoint{}, false
	}
	name := strings.TrimSpace(defaultName)
	if name == "" {
		name = fmt.Sprintf("%s-%d", proto, port)
	}
	if custom, ok := callStringArg(call, 1); ok && strings.TrimSpace(custom) != "" {
		name = strings.TrimSpace(custom)
	}
	endpoint := v1.Endpoint{Name: name, Port: port, Protocol: proto}
	endpointPublishFromCall(&endpoint, call, 2, 3)
	return endpoint, true
}

func endpointFromPortCall(call compiler.HIRCall) (v1.Endpoint, bool) {
	port, ok := callIntArg(call, 0)
	if !ok {
		return v1.Endpoint{}, false
	}
	protoStr, ok := callStringArg(call, 1)
	if !ok || strings.TrimSpace(protoStr) == "" {
		return v1.Endpoint{}, false
	}
	proto := v1.EndpointProto(strings.ToLower(strings.TrimSpace(protoStr)))
	name := fmt.Sprintf("%s-%d", proto, port)
	if custom, ok := callStringArg(call, 2); ok && strings.TrimSpace(custom) != "" {
		name = strings.TrimSpace(custom)
	}
	endpoint := v1.Endpoint{Name: name, Port: port, Protocol: proto}
	endpointPublishFromCall(&endpoint, call, 3, 4)
	return endpoint, true
}

func lowerMount(f *compiler.HIRForm) (v1.Mount, error) {
	var mount v1.Mount
	vol, ok := stringField(f, "volume")
	if !ok {
		return mount, errors.New("mount.volume is required")
	}
	mount.Volume = v1.VolumeRef{Name: vol}
	target, ok := stringField(f, "target")
	if !ok {
		return mount, errors.New("mount.target is required")
	}
	mount.Target = target
	if readOnly, ok := boolField(f, "read_only"); ok {
		mount.ReadOnly = readOnly
	}
	return mount, nil
}

func lowerEnv(f *compiler.HIRForm) (v1.EnvVar, error) {
	var env v1.EnvVar
	name, ok := stringField(f, "name")
	if !ok {
		return env, errors.New("env.name is required")
	}
	env.Name = name
	value, ok := stringField(f, "value")
	if !ok {
		return env, errors.New("env.value is required")
	}
	env.Value = value
	return env, nil
}
