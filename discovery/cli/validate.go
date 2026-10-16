package cli

import (
	"sort"
)

var hostFlags = words("server home as request-id trace-id span-id parent-span-id on-behalf-of help")

func (e *executor) ValidateFlags(path string, flags map[string]any) error {
	entry, ok := command(path)
	if !ok {
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range append(append([]string(nil), entry.Flags...), hostFlags...) {
		allowed[name] = true
	}
	var unknown []string
	for name := range flags {
		if !allowed[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		if unknown[0] == "pin" || unknown[0] == "catalog" || unknown[0] == "source" || unknown[0] == "repo" {
			return usage("%s rejects --%s; pass --dataset", path, unknown[0])
		}
		return usage("unknown flag --%s for kc %s", unknown[0], path)
	}
	for name, value := range flags {
		if _, multiple := value.([]string); multiple && path == "plugin" {
			return usage("--%s must be supplied once", name)
		}
		if _, missing := value.(bool); missing && name != "help" {
			return usage("--%s requires a value", name)
		}
	}
	return validateCoordinates(flags)
}
func validateCoordinates(flags map[string]any) error {
	repo, set := flagString(flags, "repo"), flagString(flags, "dataset")
	if repo != "" && set != "" {
		return usage("choose --repo or --dataset; do not mix")
	}
	if set != "" && (flagString(flags, "commit") != "" || flagString(flags, "ref") != "") {
		return usage("Dataset has its own fixed basis; do not mix --dataset and --commit/--ref")
	}
	if flagString(flags, "revision") != "" {
		if set == "" {
			return usage("--revision fixes a named Dataset; pass --dataset")
		}
		if revision, err := integer(flags, "revision", false); err != nil || revision == 0 {
			return usage("--revision must be a positive integer")
		}
	}
	return nil
}
func (e *executor) Validate(path string, flags map[string]any) error {
	if err := e.ValidateFlags(path, flags); err != nil {
		return err
	}
	if _, ok := command(path); !ok || flags["help"] == true {
		return nil
	}
	if path == "plugin" {
		return validatePlugin(flags)
	}
	scope := e.scope(flags)
	if scope.Dataset == "" || scope.Definition != nil {
		return usage("%s requires a named --dataset; search scope is one published Dataset revision", path)
	}
	if path == "relations" || path == "traverse" {
		if _, err := required(flags, "object"); err != nil {
			return err
		}
	}
	if _, err := integer(flags, "limit", true); err != nil {
		return err
	}
	if path == "traverse" {
		if _, err := integer(flags, "max-hops", false); err != nil {
			return err
		}
		if _, err := integer(flags, "min-hops", true); err != nil {
			return err
		}
	}
	return nil
}
