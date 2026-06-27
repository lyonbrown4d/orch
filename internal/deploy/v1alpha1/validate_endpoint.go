package v1alpha1

import (
	"net"
	"strings"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (e *Endpoint) validate() error {
	if err := e.validateName(); err != nil {
		return err
	}
	if err := validateEndpointPort("port", e.Port, true); err != nil {
		return err
	}
	if err := validateEndpointPort("hostPort", e.HostPort, false); err != nil {
		return err
	}
	if err := validateEndpointHostIP(e.HostIP); err != nil {
		return err
	}
	return e.validateProtocol()
}

func (e *Endpoint) validateName() error {
	if strings.TrimSpace(e.Name) == "" {
		return oopsx.B("deploy").Errorf("name is required")
	}
	if !nameRe.MatchString(e.Name) {
		return oopsx.B("deploy").Errorf("name is invalid: %q", e.Name)
	}
	return nil
}

func validateEndpointPort(field string, port int, required bool) error {
	if !required && port == 0 {
		return nil
	}
	if port <= 0 || port > 65535 {
		if field == "hostPort" {
			return oopsx.B("deploy").Errorf("hostPort must be 1..65535 when set (got %d)", port)
		}
		return oopsx.B("deploy").Errorf("%s must be 1..65535 (got %d)", field, port)
	}
	return nil
}

func validateEndpointHostIP(hostIP string) error {
	hostIP = strings.TrimSpace(hostIP)
	if hostIP == "" || net.ParseIP(hostIP) != nil {
		return nil
	}
	return oopsx.B("deploy").Errorf("hostIP is invalid: %q", hostIP)
}

func (e *Endpoint) validateProtocol() error {
	switch e.Protocol {
	case ProtoTCP, ProtoUDP, ProtoHTTP:
		return nil
	default:
		return oopsx.B("deploy").Errorf("invalid protocol %q", e.Protocol)
	}
}
