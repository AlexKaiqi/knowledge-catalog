package catalog

import (
	"strings"

	"kc/kernel"
)

// ValidateWorkspaceDefinition checks the shape of a consumer definition
// without opening a Repository, resolving a selector, or persisting anything.
// Replaying an existing pin must validate its recipe without following HEAD.
func ValidateWorkspaceDefinition(def WorkspaceDefinition) error {
	if def.Retired {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is retired", def.WorkspaceID)
	}
	if len(def.Sources) == 0 {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "a workspace must contain at least one repository")
	}
	for _, source := range def.Sources {
		if strings.TrimSpace(string(source.Repository)) == "" || strings.TrimSpace(source.Selector) == "" {
			return kernel.Fail(kernel.ErrWorkspaceInvalid, "a workspace source requires repository and selector")
		}
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return err
	}
	return validateSourceCoordinates(def.Sources)
}
