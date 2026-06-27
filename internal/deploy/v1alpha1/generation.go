package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const defaultNamespace = "default"

func AppGeneration(app App) string {
	copyApp := app
	copyApp.Metadata.Namespace = namespaceOrDefault(copyApp.Metadata.Namespace)
	b, err := json.Marshal(copyApp)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func namespaceOrDefault(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}
