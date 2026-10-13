package invocation

import (
	"maps"

	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

// DatasetMetadata uses a complete optional metadata object; an explicit flag
// replaces the recipe's metadata, including clearing it with {}.
func DatasetMetadata(flags map[string]invoke.FlagValue, inherited map[string]string) (map[string]string, error) {
	if _, supplied := flags["metadata"]; !supplied {
		return maps.Clone(inherited), nil
	}
	var metadata map[string]string
	if err := dataset.DecodeJSON([]byte(invoke.FlagString(flags, "metadata")), &metadata); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--metadata requires a JSON object of string values: %v", err)
	}
	return metadata, nil
}
