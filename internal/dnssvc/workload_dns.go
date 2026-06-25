package dnssvc

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type workloadAssignmentLookup interface {
	ListWorkloadAssignments() *list.List[workloadmeta.Assignment]
}

func dnsZoneName(cfg config.DNSConfig) string {
	return cfg.ZoneName()
}

func workloadRecordKey(namespace, workloadName string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	return strings.ToLower(ns) + "/" + strings.ToLower(strings.TrimSpace(workloadName))
}

// workloadServiceFQDN returns the relative owner name segments before normalization (zone is separate).
func workloadServiceFQDN(namespace, workloadName, zone string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	base := strings.Trim(strings.ToLower(zone), ".")
	return fmt.Sprintf("%s.%s.svc.%s",
		strings.ToLower(strings.TrimSpace(workloadName)),
		strings.ToLower(ns),
		base,
	)
}

func parseWorkloadServiceFQDN(name, zone string) (namespace, workload string, ok bool) {
	queryName := strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
	base := strings.Trim(strings.ToLower(strings.TrimSpace(zone)), ".")
	if queryName == "" || base == "" {
		return "", "", false
	}
	suffix := ".svc." + base
	if !strings.HasSuffix(queryName, suffix) {
		return "", "", false
	}
	left := strings.TrimSuffix(queryName, suffix)
	left = strings.Trim(left, ".")
	parts := strings.Split(left, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[1], parts[0], true
}

// UpsertWorkloadA publishes or replaces an A record for workload.<namespace>.svc.<zone>.
func (s *Service) UpsertWorkloadA(ctx context.Context, namespace, workloadName, ipv4 string) error {
	if !s.cfg.Enabled || s.store == nil {
		return nil
	}
	if strings.TrimSpace(ipv4) == "" {
		return oopsx.B("dns").Errorf("workload %q: empty ipv4", workloadName)
	}

	zone := dnsZoneName(s.cfg)
	rec := dnsserver.Record{
		Zone: zone,
		Name: workloadServiceFQDN(namespace, workloadName, zone),
		TTL:  60,
		Type: dns.TypeA,
		Data: strings.TrimSpace(ipv4),
	}
	norm, err := dnsserver.NormalizeRecord(rec)
	if err != nil {
		return oopsx.B("dns").Wrapf(err, "dns workload record")
	}

	key := workloadRecordKey(namespace, workloadName)
	prev, hadPrev := s.workloadRecords.Get(key)
	if err := s.store.SaveRecord(ctx, norm); err != nil {
		return oopsx.B("dns").Wrapf(err, "save workload record")
	}
	if hadPrev && prev.Key() != norm.Key() {
		if delErr := s.store.DeleteRecord(ctx, prev); delErr != nil {
			s.logger.Warn("delete stale dns workload record", "error", delErr)
		}
	}
	s.workloadRecords.Set(key, norm)
	s.logger.Debug("dns workload registered", "fqdn", norm.Name, "ip", ipv4)
	return nil
}

// RemoveWorkloadA deletes the A record previously registered for this workload (if any).
func (s *Service) RemoveWorkloadA(ctx context.Context, namespace, workloadName string) error {
	if !s.cfg.Enabled || s.store == nil {
		return nil
	}
	key := workloadRecordKey(namespace, workloadName)
	prev, ok := s.workloadRecords.Get(key)
	if !ok {
		return nil
	}
	if err := s.store.DeleteRecord(ctx, prev); err != nil {
		return oopsx.B("dns").Wrapf(err, "delete workload record")
	}
	s.workloadRecords.Delete(key)
	s.logger.Debug("dns workload deregistered", "workload", workloadName, "namespace", namespace)
	return nil
}

func (s *Service) lookupLocalWorkloadIPv4(namespace, workloadName string) (string, bool) {
	key := workloadRecordKey(namespace, workloadName)
	rec, ok := s.workloadRecords.Get(key)
	if !ok {
		return "", false
	}
	ip := strings.TrimSpace(rec.Data)
	if net.ParseIP(ip).To4() == nil {
		return "", false
	}
	return ip, true
}

// LookupLocalWorkloadIPv4 returns only the record registered in this DNS service.
func (s *Service) LookupLocalWorkloadIPv4(namespace, workloadName string) (string, bool) {
	if s == nil {
		return "", false
	}
	return s.lookupLocalWorkloadIPv4(namespace, workloadName)
}

// LookupWorkloadIPv4 returns the last registered A record data for workload DNS.
// It first checks local runtime registrations, then falls back to replicated assignment addresses.
func (s *Service) LookupWorkloadIPv4(namespace, workloadName string) (string, bool) {
	if s == nil {
		return "", false
	}
	if ip, ok := s.lookupLocalWorkloadIPv4(namespace, workloadName); ok {
		return ip, true
	}
	return s.lookupAssignmentWorkloadIPv4(namespace, workloadName)
}

func (s *Service) lookupAssignmentWorkloadIPv4(namespace, workloadName string) (string, bool) {
	if s == nil || s.assignments == nil {
		return "", false
	}
	ns := workloadmeta.NamespaceOrDefault(namespace)
	name := strings.TrimSpace(workloadName)
	if name == "" {
		return "", false
	}
	assignments := s.assignments.ListWorkloadAssignments()
	var out string
	assignments.Range(func(_ int, assignment workloadmeta.Assignment) bool {
		if workloadmeta.NamespaceOrDefault(assignment.Metadata.Namespace) != ns {
			return true
		}
		if strings.TrimSpace(assignment.Workload) != name {
			return true
		}
		if strings.TrimSpace(assignment.Status) != workloadmeta.AssignmentStatusRunning {
			return true
		}
		address := strings.TrimSpace(assignment.Address)
		if net.ParseIP(address).To4() == nil {
			return true
		}
		out = address
		return false
	})
	return out, out != ""
}
