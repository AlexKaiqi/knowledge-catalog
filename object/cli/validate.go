package cli

import (
	objectmodel "kc/object"
	"sort"
	"strings"
)

var hostFlags = words("server home as request-id trace-id span-id parent-span-id on-behalf-of help")
var repeatedFlags = map[string]bool{"source-ref": true, "evidence-ref": true}

func referenceCommand(path string) bool {
	return path == "read" || path == "writer put" || path == "writer remove"
}

func (e *executor) ValidateFlags(path string, flags map[string]any) error {
	entry, ok := command(path)
	if !ok {
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range append(append([]string(nil), entry.Flags...), hostFlags...) {
		allowed[name] = true
	}
	if referenceCommand(path) {
		if value, supplied := flags["reference"]; supplied {
			if _, bound := value.(operandValue); !bound {
				return usage("kc %s rejects --reference; supply the object URN as its positional argument", path)
			}
			allowed["reference"] = true
		}
	}
	var unknown []string
	for name := range flags {
		if !allowed[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		name := unknown[0]
		if referenceCommand(path) {
			switch name {
			case "object", "aspect", "member", "include", "exclude", "dataset", "pin", "catalog", "source":
				return usage("kc %s rejects --%s; select the object with an object URN and optional locator", path, name)
			}
		}
		return usage("unknown flag --%s for kc %s", name, path)
	}
	for name, value := range flags {
		if _, multiple := value.([]string); multiple && !repeatedFlags[name] {
			return usage("--%s must be supplied once", name)
		}
		if _, missing := value.(bool); missing && name != "help" && name != "if-absent" {
			return usage("--%s requires a value", name)
		}
	}
	return validateCoordinates(flags)
}

func (e *executor) Validate(path string, flags map[string]any) error {
	if err := e.ValidateFlags(path, flags); err != nil {
		return err
	}
	if _, ok := command(path); !ok || flags["help"] == true {
		return nil
	}
	reference := flagString(flags, "reference")
	repo := flagString(flags, "repo")
	set := flagString(flags, "dataset")
	if referenceCommand(path) {
		if strings.TrimSpace(reference) == "" {
			return usage("kc %s requires an object URN as its positional argument", path)
		}
	} else if path != "schema list" {
		if _, err := required(flags, "object"); err != nil {
			return err
		}
	}
	if strings.HasPrefix(path, "writer ") {
		if _, err := required(flags, "command-id"); err != nil {
			return err
		}
		if flagBool(flags, "if-absent") && flagString(flags, "if-digest") != "" {
			return usage("use only one of --if-absent or --if-digest")
		}
		if path == "writer put" {
			_, err := jsonValue(flags)
			return err
		}
		return nil
	}
	if reference == "" && repo == "" && set == "" && (path == "schema list" || e.context.Dataset == "") {
		return usage("missing --repo or --dataset")
	}
	_, err := limitFrom(flags)
	return err
}

func validateCoordinates(flags map[string]any) error {
	reference := flagString(flags, "reference")
	repo := flagString(flags, "repo")
	set := flagString(flags, "dataset")
	if reference != "" {
		if _, err := objectmodel.ParseReference(reference); err != nil {
			return err
		}
	} else {
		if flagString(flags, "member") != "" && flagString(flags, "aspect") == "" {
			return usage("member address requires --aspect and --member")
		}
	}
	if repo != "" && set != "" {
		return usage("choose --repo or --dataset; do not mix")
	}
	if set != "" && flagString(flags, "ref") != "" {
		return usage("Dataset has its own fixed basis; do not pass --ref")
	}
	if reference == "" && set != "" && flagString(flags, "commit") != "" {
		return usage("Dataset has its own fixed basis; do not mix --dataset and --commit without an object reference")
	}
	if flagString(flags, "commit") != "" && flagString(flags, "ref") != "" {
		return usage("choose --commit or --ref; do not mix")
	}
	return nil
}
