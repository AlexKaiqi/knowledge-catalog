package config

import (
	"fmt"
	"regexp"
	"strings"
)

var unsafeRepoChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

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
		if !strings.HasPrefix(raw, "kr://") {
			raw = "kr://" + raw
		}
		rest := strings.TrimPrefix(raw, "kr://")
		if strings.Count(rest, "/") < 1 || strings.HasSuffix(rest, "/") || strings.ContainsAny(rest, " \r\n") {
			return "", fmt.Errorf("repository id must be kr://<org>/<name>")
		}
		return raw, nil
	}
	if ValidExternalAuthorityName(raw) {
		return raw, nil
	}
	return "", fmt.Errorf("repository id must be kr://<org>/<name> or a LakeFS Graveler name, got %q", raw)
}

// EncodeRepoDir maps a kr:// id to one directory name. Lossy on purpose: the
// authoritative id is stamped inside the directory, never read back off the path.
func EncodeRepoDir(repositoryID string) string {
	return unsafeRepoChars.ReplaceAllString(repositoryID, "_")
}
