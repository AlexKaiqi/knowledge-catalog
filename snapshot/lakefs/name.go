package lakefs

import (
	"strings"
	"unicode"

	"kc/kernel"
)

// lakeFS 1.86 RepositoryCreation.name: ^[a-z0-9][a-z0-9-]{2,62}$
const gravelerNameMax = 63

func ValidGravelerName(name string) bool {
	if len(name) < 3 || len(name) > gravelerNameMax {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i > 0 {
			continue
		}
		return false
	}
	return true
}

func PlatformGravelerName(name string) bool {
	return name == "kc-catalog" || name == "kc-system" || strings.HasPrefix(name, "kc-")
}

func GravelerSlug(raw string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			hyphen = false
		case r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
			if b.Len() > 0 && !hyphen {
				b.WriteByte('-')
				hyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// ManagedGravelerName is the lakeFS repository id for a managed business
// Snapshot. It is the slug of --name. Owner is not part of the Graveler id.
// kc- is reserved for platform repositories (kc-catalog, kc-system).
// Allocation is the ledger/storage token, not the visible Graveler id.
func ManagedGravelerName(name, owner, allocation string) (string, error) {
	nameSlug := GravelerSlug(name)
	if strings.TrimSpace(name) != "" && nameSlug == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "LakeFS repository names must be lowercase letters, digits, and hyphens (3-63 characters); %q cannot be a Graveler id", name)
	}
	if nameSlug != "" && PlatformGravelerName(nameSlug) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "kc- is reserved for platform repositories (kc-catalog, kc-system)")
	}
	if nameSlug == "" {
		return unnamedGravelerName(allocation)
	}
	candidate := fitGravelerName(nameSlug)
	if !ValidGravelerName(candidate) || PlatformGravelerName(candidate) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "LakeFS repository name %q is not a usable Graveler id", candidate)
	}
	return candidate, nil
}

func unnamedGravelerName(allocation string) (string, error) {
	alloc := strings.ToLower(strings.TrimSpace(allocation))
	if !validAllocationToken(alloc) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "managed allocation identity is required")
	}
	short := alloc
	if len(short) > 12 {
		short = short[:12]
	}
	candidate := fitGravelerName("repo-" + short)
	if !ValidGravelerName(candidate) || PlatformGravelerName(candidate) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "LakeFS repository name %q is not a usable Graveler id", candidate)
	}
	return candidate, nil
}

func validAllocationToken(allocation string) bool {
	if allocation == "" {
		return false
	}
	for i := 0; i < len(allocation); i++ {
		c := allocation[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func fitGravelerName(name string) string {
	name = strings.Trim(name, "-")
	if len(name) > gravelerNameMax {
		name = strings.Trim(name[:gravelerNameMax], "-")
	}
	return name
}
