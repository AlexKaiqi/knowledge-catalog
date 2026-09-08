// Package identity owns the service's stable username and external identity
// binding. It is independent of Catalog membership and authorization grants.
package identity

import (
	"fmt"
	"strings"
)

// CanonicalUsername validates a portable account identifier. It never silently
// renames an identity: casing, whitespace, and unsupported characters fail.
// The same value is used by KC authorization and managed Snapshot accounts.
func CanonicalUsername(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 39 || raw != strings.TrimSpace(raw) || raw != strings.ToLower(raw) {
		return "", fmt.Errorf("username must be 1–39 lowercase ASCII characters")
	}
	separator := false
	for i, c := range raw {
		alphanumeric := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if alphanumeric {
			separator = false
			continue
		}
		if (c != '-' && c != '_' && c != '.') || i == 0 || i == len(raw)-1 || separator {
			return "", fmt.Errorf("username must use letters or digits, with single internal -, _ or . separators")
		}
		separator = true
	}
	return raw, nil
}
