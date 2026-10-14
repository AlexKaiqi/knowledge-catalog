package cli

import (
	"fmt"
	"kc/kernel"
	"strconv"
	"strings"
)

func flagString(flags map[string]any, name string) string {
	values := flagStrings(flags, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
func flagStrings(flags map[string]any, name string) []string {
	switch value := flags[name].(type) {
	case nil, bool:
		return nil
	case []string:
		return value
	default:
		return []string{fmt.Sprint(value)}
	}
}
func usage(format string, args ...any) error {
	return kernel.Fail(kernel.ErrUsageInvalid, format, args...)
}
func required(flags map[string]any, name string) (string, error) {
	value := strings.TrimSpace(flagString(flags, name))
	if value == "" {
		return "", usage("missing --%s", name)
	}
	return value, nil
}
func integer(flags map[string]any, name string, optional bool) (int, error) {
	raw := strings.TrimSpace(flagString(flags, name))
	if raw == "" && optional {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, usage("--%s must be a non-negative integer", name)
	}
	return value, nil
}
