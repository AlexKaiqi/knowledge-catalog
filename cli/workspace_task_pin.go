package cli

import (
	"encoding/json"
	"os"
	"strings"

	"kc/catalog"
	"kc/kernel"
)

const workspaceDefinitionFlag = "_workspace-definition"

// taskWorkspacePin is a client-owned, portable task input. It keeps the
// ResolvedWorkspace fields intact and adds the unpublished recipe needed to
// replay its membership and layout. It is never a Catalog registry object.
type taskWorkspacePin struct {
	catalog.ResolvedWorkspace
	Catalog    string                       `json:"catalog,omitempty"`
	Definition *catalog.WorkspaceDefinition `json:"definition,omitempty"`
}

func suppliedWorkspaceDefinition(flags map[string]FlagValue) *catalog.WorkspaceDefinition {
	def, _ := flags[workspaceDefinitionFlag].(*catalog.WorkspaceDefinition)
	return def
}

func prepareRemoteKnowledgeContext(flags map[string]FlagValue) error {
	if FlagString(flags, "repo") != "" {
		return nil
	}
	var definition *catalog.WorkspaceDefinition
	if path := FlagString(flags, "workspace-file"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		recipe, err := catalog.ParseWorkspaceRecipe(raw)
		if err != nil {
			return err
		}
		definition = &catalog.WorkspaceDefinition{Revision: 1, Sources: recipe.Sources()}
	}
	if raw := strings.TrimSpace(FlagString(flags, "pin")); raw != "" {
		if !strings.HasPrefix(raw, "{") {
			content, err := os.ReadFile(raw)
			if err != nil {
				return err
			}
			raw = string(content)
		}
		var saved taskWorkspacePin
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
				return kernel.Fail(kernel.ErrUsageInvalid, "--workspace-file does not match the definition saved in --pin")
			}
			definition = saved.Definition
		}
		pin, err := json.Marshal(saved.ResolvedWorkspace)
		if err != nil {
			return err
		}
		flags["pin"] = string(pin)
		if definition == nil && FlagString(flags, "workspace") == "" && saved.WorkspaceID != "" {
			flags["workspace"] = saved.WorkspaceID
		}
	}
	if definition != nil {
		if FlagString(flags, "workspace") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --workspace or a temporary definition")
		}
		if err := catalog.ValidateWorkspaceDefinition(*definition); err != nil {
			return err
		}
		flags[workspaceDefinitionFlag] = definition
	}
	return nil
}
