package invocation

import (
	"encoding/json"
	"os"
	"strings"

	"kc/catalog"
	"kc/invoke"
	"kc/kernel"
)

const WorkspaceDefinitionFlag = "_workspace-definition"

// TaskKnowledgeSetPin is a client-owned, portable task input. It keeps the
// ResolvedKnowledgeSet fields intact and adds the unpublished recipe needed to
// replay its membership and layout. It is never a Catalog registry object.
type TaskKnowledgeSetPin struct {
	catalog.ResolvedKnowledgeSet
	Catalog    string                `json:"catalog,omitempty"`
	Definition *catalog.KnowledgeSet `json:"definition,omitempty"`
}

func SuppliedKnowledgeSet(flags map[string]invoke.FlagValue) *catalog.KnowledgeSet {
	def, _ := flags[WorkspaceDefinitionFlag].(*catalog.KnowledgeSet)
	return def
}

// hoistTaskPinDefinition lifts an embedded temporary recipe out of --pin before
// mixed-basis checks or authorization run. It is safe to call repeatedly.
func HoistTaskPinDefinition(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "repo") != "" || SuppliedKnowledgeSet(flags) != nil {
		return nil
	}
	raw := strings.TrimSpace(invoke.FlagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		return nil
	}
	var saved TaskKnowledgeSetPin
	if err := catalog.DecodeJSON([]byte(raw), &saved); err != nil {
		return nil
	}
	if saved.Catalog != "" {
		if selected := invoke.FlagString(flags, "catalog"); selected != "" && selected != saved.Catalog {
			return kernel.Fail(kernel.ErrUsageInvalid, "--catalog does not match the Catalog saved in --pin")
		}
		flags["catalog"] = saved.Catalog
	}
	if saved.Definition == nil {
		return nil
	}
	if invoke.FlagString(flags, "dataset") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
	}
	if err := catalog.ValidateKnowledgeSet(*saved.Definition); err != nil {
		return err
	}
	flags[WorkspaceDefinitionFlag] = saved.Definition
	delete(flags, "dataset")
	pin, err := json.Marshal(saved.ResolvedKnowledgeSet)
	if err != nil {
		return err
	}
	flags["pin"] = string(pin)
	return nil
}

// PrepareKnowledgePinContext normalizes --pin and --dataset-file into the
// internal coordinates openServing expects. Remote CLI and embedded application
// paths both call this before knowledge verbs run.
func PrepareKnowledgePinContext(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "repo") != "" {
		return nil
	}
	if err := HoistTaskPinDefinition(flags); err != nil {
		return err
	}
	var definition *catalog.KnowledgeSet
	if supplied := SuppliedKnowledgeSet(flags); supplied != nil {
		definition = supplied
	}
	if path := invoke.FlagString(flags, "dataset-file"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		recipe, err := catalog.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return err
		}
		definition = &catalog.KnowledgeSet{Revision: 1, Sources: recipe.Sources()}
	}
	if raw := strings.TrimSpace(invoke.FlagString(flags, "pin")); raw != "" && definition == nil {
		if !strings.HasPrefix(raw, "{") {
			content, err := os.ReadFile(raw)
			if err != nil {
				return err
			}
			raw = string(content)
		}
		var saved TaskKnowledgeSetPin
		if err := catalog.DecodeJSON([]byte(raw), &saved); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a valid task pin: %v", err)
		}
		if saved.Catalog != "" {
			if selected := invoke.FlagString(flags, "catalog"); selected != "" && selected != saved.Catalog {
				return kernel.Fail(kernel.ErrUsageInvalid, "--catalog does not match the Catalog saved in --pin")
			}
			flags["catalog"] = saved.Catalog
		}
		if saved.Definition != nil {
			if definition != nil && kernel.CanonicalDigest(definition) != kernel.CanonicalDigest(saved.Definition) {
				return kernel.Fail(kernel.ErrUsageInvalid, "--dataset-file does not match the definition saved in --pin")
			}
			definition = saved.Definition
		}
		pin, err := json.Marshal(saved.ResolvedKnowledgeSet)
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
			case "knowledge.search", "knowledge.rerank":
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			default:
				return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
			}
		}
		if err := catalog.ValidateKnowledgeSet(*definition); err != nil {
			return err
		}
		flags[WorkspaceDefinitionFlag] = definition
		delete(flags, "dataset")
	}
	return nil
}
