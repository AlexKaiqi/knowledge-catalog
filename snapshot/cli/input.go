package cli

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/snapshot/dataset"
)

const (
	defaultHistoryLimit = 50
	maxHistoryPageSize  = 200
)

func flagString(flags map[string]any, name string) string {
	values := flagStrings(flags, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func flagStrings(flags map[string]any, name string) []string {
	value, ok := flags[name]
	if !ok {
		return nil
	}
	switch value := value.(type) {
	case bool:
		return nil
	case []string:
		return value
	default:
		return []string{fmt.Sprint(value)}
	}
}

func requireRemoteFlag(flags map[string]any, name string) (string, error) {
	value := strings.TrimSpace(flagString(flags, name))
	if value == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "missing --%s", name)
	}
	return value, nil
}

func remoteIntFlag(flags map[string]any, name string) (int, error) {
	raw, err := requireRemoteFlag(flags, name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--%s must be an integer", name)
	}
	return value, nil
}

func remoteLimit(flags map[string]any) (int, error) {
	if strings.TrimSpace(flagString(flags, "limit")) == "" {
		return 0, nil
	}
	value, err := remoteIntFlag(flags, "limit")
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--limit must be a non-negative integer")
	}
	return value, nil
}

func pageLimit(flags map[string]any, fallback, maximum int) (int, error) {
	value, err := remoteLimit(flags)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		value = fallback
	}
	if value > maximum {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "page limit cannot exceed %d", maximum)
	}
	return value, nil
}

func snapshotRevision(flags map[string]any) (*int, error) {
	if _, present := flags["revision"]; !present {
		return nil, nil
	}
	value, err := remoteIntFlag(flags, "revision")
	if err != nil {
		return nil, err
	}
	if value <= 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--revision must be a positive published version")
	}
	return &value, nil
}

func datasetMetadata(flags map[string]any, inherited map[string]string) (map[string]string, error) {
	if _, supplied := flags["metadata"]; !supplied {
		return maps.Clone(inherited), nil
	}
	var metadata map[string]string
	if err := dataset.DecodeJSON([]byte(flagString(flags, "metadata")), &metadata); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--metadata requires a JSON object of string values: %v", err)
	}
	return metadata, nil
}

func validRelativePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func splitCmds(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func rejectFlagsOutside(flags map[string]any, allowed map[string]struct{}, where string) error {
	var unknown []string
	for name := range flags {
		if _, ok := allowed[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "--%s is not valid for %s", unknown[0], where)
	}
	return nil
}
