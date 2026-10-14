package invocation

import (
	"encoding/json"
	"os"
	"strings"

	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

const WorkspaceDefinitionFlag = "_workspace-definition"

// TaskDatasetPin is a client-owned, portable task input. It keeps the
// ResolvedDataset fields intact and adds the unpublished recipe needed to
// replay its membership and layout. It is never a Registry registry object.
type TaskDatasetPin struct {
	dataset.ResolvedDataset
	Definition *dataset.Dataset `json:"definition,omitempty"`
}

func SuppliedDataset(flags map[string]invoke.FlagValue) *dataset.Dataset {
	def, _ := flags[WorkspaceDefinitionFlag].(*dataset.Dataset)
	return def
}

// hoistTaskPinDefinition lifts an embedded temporary recipe out of --pin before
// mixed-basis checks or authorization run. It is safe to call repeatedly.
func HoistTaskPinDefinition(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "repo") != "" || SuppliedDataset(flags) != nil {
		return nil
	}
	raw := strings.TrimSpace(invoke.FlagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		return nil
	}
	var saved TaskDatasetPin
	if err := dataset.DecodeJSON([]byte(raw), &saved); err != nil {
		return nil
	}
	if saved.Definition == nil {
		return nil
	}
	if invoke.FlagString(flags, "dataset") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
	}
	if err := dataset.ValidateDataset(*saved.Definition); err != nil {
		return err
	}
	flags[WorkspaceDefinitionFlag] = saved.Definition
	delete(flags, "dataset")
	pin, err := json.Marshal(saved.ResolvedDataset)
	if err != nil {
		return err
	}
	flags["pin"] = string(pin)
	return nil
}

// PrepareObjectPinContext normalizes --pin and --dataset-file into the
// internal coordinates openServing expects. Remote CLI and embedded application
// paths both call this before object verbs run.
func PrepareObjectPinContext(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "repo") != "" {
		return nil
	}
	if err := HoistTaskPinDefinition(flags); err != nil {
		return err
	}
	var definition *dataset.Dataset
	if supplied := SuppliedDataset(flags); supplied != nil {
		definition = supplied
	}
	if path := invoke.FlagString(flags, "dataset-file"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		recipe, err := dataset.ParseDatasetRecipe(raw)
		if err != nil {
			return err
		}
		definition = &dataset.Dataset{Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata}
	}
	if raw := strings.TrimSpace(invoke.FlagString(flags, "pin")); raw != "" && definition == nil {
		if !strings.HasPrefix(raw, "{") {
			content, err := os.ReadFile(raw)
			if err != nil {
				return err
			}
			raw = string(content)
		}
		var saved TaskDatasetPin
		if err := dataset.DecodeJSON([]byte(raw), &saved); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a valid task pin: %v", err)
		}
		if saved.Definition != nil {
			if definition != nil && kernel.CanonicalDigest(definition) != kernel.CanonicalDigest(saved.Definition) {
				return kernel.Fail(kernel.ErrUsageInvalid, "--dataset-file does not match the definition saved in --pin")
			}
			definition = saved.Definition
		}
		pin, err := json.Marshal(saved.ResolvedDataset)
		if err != nil {
			return err
		}
		flags["pin"] = string(pin)
		if definition == nil && invoke.FlagString(flags, "dataset") == "" && saved.SetID != "" {
			flags["dataset"] = saved.SetID
		}
	}
	if definition != nil {
		if invoke.FlagString(flags, "dataset") != "" {
			switch invoke.FlagString(flags, "_action") {
			case "object.search", "object.rerank":
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			default:
				return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
			}
		}
		if err := dataset.ValidateDataset(*definition); err != nil {
			return err
		}
		flags[WorkspaceDefinitionFlag] = definition
		delete(flags, "dataset")
	}
	return nil
}
