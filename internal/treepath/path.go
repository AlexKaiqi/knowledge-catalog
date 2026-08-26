// Package treepath validates opaque Snapshot tree paths.
package treepath

import (
	"os"
	"path/filepath"
	"strings"

	"kc/kernel"
)

func Clean(value string) (string, error) {
	if value == "" || filepath.IsAbs(value) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "path must be relative: %s", value)
	}
	normalized := filepath.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(os.PathSeparator)) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "path escapes repository root: %s", value)
	}
	return normalized, nil
}
