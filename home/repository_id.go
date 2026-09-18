package home

import (
	"fmt"
	"strings"

	"kc/snapshot/lakefs"
)

// NormalizeRepositoryID accepts kr://<org>/<name> or a lakeFS Graveler name.
// LakeFS managed create uses the Graveler id as the protocol id so --repo and
// the lakeFS repository are the same name. Platform Graveler ids (kc-catalog,
// kc-system, kc-*) stay reserved.
func NormalizeRepositoryID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("repository id is required")
	}
	if strings.Contains(raw, "/") || strings.Contains(raw, "://") {
		return NormalizeCatalogID(raw)
	}
	if lakefs.ValidGravelerName(raw) && !lakefs.PlatformGravelerName(raw) {
		return raw, nil
	}
	return "", fmt.Errorf("repository id must be kr://<org>/<name> or a LakeFS Graveler name, got %q", raw)
}
