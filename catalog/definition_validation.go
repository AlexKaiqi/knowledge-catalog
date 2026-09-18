package catalog

import (
	"strings"

	"kc/kernel"
)

// ValidateKnowledgeSet checks the shape of a consumer definition
// without opening a Repository, resolving a selector, or persisting anything.
// Replaying an existing pin must validate its recipe without following HEAD.
func ValidateKnowledgeSet(def KnowledgeSet) error {
	if def.Retired {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "workspace %s is retired", def.SetID)
	}
	if len(def.Sources) == 0 {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "a workspace must contain at least one repository")
	}
	for _, source := range def.Sources {
		if strings.TrimSpace(string(source.Repository)) == "" || strings.TrimSpace(source.Selector) == "" {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "a workspace source requires repository and selector")
		}
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return err
	}
	return validateSourceCoordinates(def.Sources)
}
