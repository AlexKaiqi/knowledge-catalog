package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"kc/kernel"
	"os"
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
func flagBool(flags map[string]any, name string) bool {
	return flags[name] == true || flagString(flags, name) == "true"
}
func required(flags map[string]any, name string) (string, error) {
	value := strings.TrimSpace(flagString(flags, name))
	if value == "" {
		return "", usage("missing --%s", name)
	}
	return value, nil
}
func usage(format string, args ...any) error {
	return kernel.Fail(kernel.ErrUsageInvalid, format, args...)
}
func limitFrom(flags map[string]any) (int, error) {
	raw := flagString(flags, "limit")
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, usage("--limit must be a non-negative integer")
	}
	return value, nil
}

// revisionFrom reads --revision: a published Dataset revision that fixes every
// member commit for this call, so consecutive calls see one consistent basis.
func revisionFrom(flags map[string]any) (int, error) {
	raw := flagString(flags, "revision")
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, usage("--revision must be a positive integer")
	}
	return value, nil
}
func jsonValue(flags map[string]any) (any, error) {
	raw := flagString(flags, "value")
	file := flagString(flags, "file")
	if raw != "" && file != "" {
		return nil, usage("use only one of --file or --value")
	}
	if file != "" {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		raw = string(content)
	}
	if raw == "" {
		return nil, usage("put requires --file or --value")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, usage("invalid JSON value: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, usage("value requires exactly one JSON value")
	}
	return value, nil
}
