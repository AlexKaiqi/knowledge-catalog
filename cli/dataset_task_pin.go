package cli

import (
	"encoding/json"
	"os"
	"strings"

	"kc/catalog"
	"kc/kernel"
)

const workspaceDefinitionFlag = "_workspace-definition"

// taskKnowledgeSetPin is a client-owned, portable task input. It keeps the
// ResolvedKnowledgeSet fields intact and adds the unpublished recipe needed to
// replay its membership and layout. It is never a Catalog registry object.
type taskKnowledgeSetPin struct {
	catalog.ResolvedKnowledgeSet
	Catalog    string                `json:"catalog,omitempty"`
	Definition *catalog.KnowledgeSet `json:"definition,omitempty"`
}

func suppliedKnowledgeSet(flags map[string]FlagValue) *catalog.KnowledgeSet {
	def, _ := flags[workspaceDefinitionFlag].(*catalog.KnowledgeSet)
	return def
}

// hoistTaskPinDefinition lifts an embedded temporary recipe out of --pin before
// mixed-basis checks or authorization run. It is safe to call repeatedly.
func hoistTaskPinDefinition(flags map[string]FlagValue) error {
	if FlagString(flags, "repo") != "" || suppliedKnowledgeSet(flags) != nil {
		return nil
	}
	raw := strings.TrimSpace(FlagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		return nil
	}
	var saved taskKnowledgeSetPin
	if err := catalog.DecodeJSON([]byte(raw), &saved); err != nil {
		return nil
	}
	if saved.Catalog != "" {
		if selected := FlagString(flags, "catalog"); selected != "" && selected != saved.Catalog {
			return kernel.Fail(kernel.ErrUsageInvalid, "--catalog does not match the Catalog saved in --pin")
		}
		flags["catalog"] = saved.Catalog
	}
	if saved.Definition == nil {
		return nil
	}
	if FlagString(flags, "dataset") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
	}
	if err := catalog.ValidateKnowledgeSet(*saved.Definition); err != nil {
		return err
	}
	flags[workspaceDefinitionFlag] = saved.Definition
	delete(flags, "dataset")
	pin, err := json.Marshal(saved.ResolvedKnowledgeSet)
	if err != nil {
		return err
	}
	flags["pin"] = string(pin)
	return nil
}

// prepareKnowledgePinContext normalizes --pin and --dataset-file into the
// internal coordinates openServing expects. Remote CLI and embedded application
// paths both call this before knowledge verbs run.
func prepareKnowledgePinContext(flags map[string]FlagValue) error {
	if FlagString(flags, "repo") != "" {
		return nil
	}
	if err := hoistTaskPinDefinition(flags); err != nil {
		return err
	}
	var definition *catalog.KnowledgeSet
	if supplied := suppliedKnowledgeSet(flags); supplied != nil {
		definition = supplied
	}
	if path := FlagString(flags, "dataset-file"); path != "" {
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
	if raw := strings.TrimSpace(FlagString(flags, "pin")); raw != "" && definition == nil {
		if !strings.HasPrefix(raw, "{") {
			content, err := os.ReadFile(raw)
			if err != nil {
				return err
			}
			raw = string(content)
		}
		var saved taskKnowledgeSetPin
		if err := catalog.DecodeJSON([]byte(raw), &saved); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a valid task pin: %v", err)
		}
		if saved.Catalog != "" {
			if selected := FlagString(flags, "catalog"); selected != "" && selected != saved.Catalog {
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
		if definition == nil && FlagString(flags, "dataset") == "" && saved.SetID != "" {
			flags["dataset"] = saved.SetID
		}
	}
	if definition != nil {
		if FlagString(flags, "dataset") != "" {
			switch FlagString(flags, "_action") {
			case "knowledge.search", "knowledge.rerank":
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			default:
				return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
			}
		}
		if err := catalog.ValidateKnowledgeSet(*definition); err != nil {
			return err
		}
		flags[workspaceDefinitionFlag] = definition
		delete(flags, "dataset")
	}
	return nil
}
