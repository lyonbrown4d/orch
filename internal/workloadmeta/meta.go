package workloadmeta

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/lo"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

// NamespaceOrDefault returns a non-empty namespace for DNS and naming.
func NamespaceOrDefault(ns string) string {
	return lo.CoalesceOrEmpty(strings.TrimSpace(ns), "default")
}

// SanitizeName maps arbitrary strings to container-friendly identifiers.
func SanitizeName(s string) string {
	var buf []byte
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			buf = utf8.AppendRune(buf, r)
		case r == '.' || r == '_' || r == '-':
			buf = utf8.AppendRune(buf, r)
		default:
			buf = append(buf, '-')
		}
	}
	out := strings.Trim(string(buf), "-")
	if out == "" {
		return "x"
	}
	return out
}

// OrchContainerName is a stable container name derived from app metadata + workload.
func OrchContainerName(meta deployv1.Metadata, workload string) string {
	ns := NamespaceOrDefault(meta.Namespace)
	app := strings.TrimSpace(meta.Name)
	w := strings.TrimSpace(workload)
	return fmt.Sprintf("orch-%s-%s-%s", SanitizeName(ns), SanitizeName(app), SanitizeName(w))
}

// LabelMap returns container labels for lookup on stop/diagnostics.
func LabelMap(meta deployv1.Metadata, w deployv1.Workload) *mapping.Map[string, string] {
	labels := mapping.NewMapWithCapacity[string, string](4)
	labels.Set("orch.io/app", meta.Name)
	labels.Set("orch.io/namespace", NamespaceOrDefault(meta.Namespace))
	labels.Set("orch.io/workload", w.Name)
	labels.Set("orch.io/runtime", string(w.Runtime))
	return labels
}

// Labels are applied to containers for lookup on stop/diagnostics.
func Labels(meta deployv1.Metadata, w deployv1.Workload) map[string]string {
	return LabelMap(meta, w).All()
}

// NormalizeImageRef expands short docker.io/library names.
func NormalizeImageRef(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return image
	}
	if strings.Contains(image, "/") {
		return image
	}
	return "docker.io/library/" + image
}
