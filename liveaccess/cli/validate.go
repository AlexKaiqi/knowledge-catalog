package cli

import (
	"encoding/json"
	"slices"
	"strings"

	"kc/kernel"
)

func (e *executor) ValidateFlags(path string, flags map[string]any) error {
	entry, supported := command(path)
	if !supported {
		return nil
	}
	allowed := append(slices.Clone(entry.Flags), strings.Fields("server home as request-id trace-id span-id parent-span-id on-behalf-of help")...)
	names := make([]string, 0, len(flags))
	for name := range flags {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if name == "pin" || name == "catalog" || name == "source" {
			return kernel.Fail(kernel.ErrUsageInvalid, "kc %s rejects --%s", path, name)
		}
		if !slices.Contains(allowed, name) {
			return kernel.Fail(kernel.ErrUsageInvalid, "unknown flag --%s for kc %s", name, path)
		}
		if _, repeated := flags[name].([]string); repeated {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s must be supplied once", name)
		}
		if _, missing := flags[name].(bool); missing && name != "help" {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s requires a value", name)
		}
	}
	return nil
}

// InputFlags projects an already validated host invocation onto this module's
// command operands. Internal task state is supplied through Context instead.
func InputFlags(path string, flags map[string]any) map[string]any {
	entry, ok := command(path)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(entry.Flags))
	for _, name := range append(slices.Clone(entry.Flags), "help") {
		if value, exists := flags[name]; exists {
			out[name] = value
		}
	}
	return out
}

func (e *executor) Validate(path string, flags map[string]any) error {
	if err := e.ValidateFlags(path, flags); err != nil {
		return err
	}
	if _, supported := command(path); !supported || flags["help"] == true {
		return nil
	}
	if err := e.validateBasis(flags); err != nil {
		return err
	}
	required := []string{"object", "aspect"}
	if path == "invoke" {
		required = []string{"object", "operation", "input"}
	}
	for _, name := range required {
		if flagString(flags, name) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "missing --%s", name)
		}
	}
	if path == "invoke" && !json.Valid([]byte(flagString(flags, "input"))) {
		return kernel.Fail(kernel.ErrUsageInvalid, "--input must be one JSON value")
	}
	return nil
}

func (e *executor) validateBasis(flags map[string]any) error {
	repo, dataset := flagString(flags, "repo"), flagString(flags, "dataset")
	if repo != "" && dataset != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose --repo or --dataset; do not mix")
	}
	if repo == "" && dataset == "" && e.context.Scope.Dataset == "" && len(e.context.Scope.Pin) == 0 && e.context.Scope.Definition == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "missing --repo or --dataset")
	}
	if repo == "" && (flagString(flags, "commit") != "" || flagString(flags, "ref") != "") {
		return kernel.Fail(kernel.ErrUsageInvalid, "Dataset has its own fixed basis; do not pass --commit or --ref")
	}
	if flagString(flags, "commit") != "" && flagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose --commit or --ref; do not mix")
	}
	return nil
}
