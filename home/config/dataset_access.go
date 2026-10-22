package config

import (
	"strings"

	"kc/kernel"
)

// DatasetAccess is a deployment-declared default for authenticated principals
// on one published Dataset: every verified caller may consume that delivery.
// It never admits the source Repositories, writes, grants or Dataset
// management, and it is not a grant to a synthetic principal.
type DatasetAccess struct {
	ID                   string   `json:"id" yaml:"id"`
	AuthenticatedActions []string `json:"authenticatedActions,omitempty" yaml:"authenticatedActions,omitempty"`
}

// AuthenticatedDatasetActions is the closed consume-side set a Dataset may
// grant to every authenticated principal.
func AuthenticatedDatasetActions() []string {
	return []string{"dataset.resolve", "file.read"}
}

// ValidateDatasetAccess refuses malformed, duplicate or empty declarations.
func ValidateDatasetAccess(items []DatasetAccess) error {
	allowed := map[string]bool{}
	for _, action := range AuthenticatedDatasetActions() {
		allowed[action] = true
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || strings.TrimSpace(item.ID) != item.ID || strings.ContainsAny(item.ID, " \t\n/") {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess has invalid Dataset id %q", item.ID)
		}
		if seen[item.ID] {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess has duplicate Dataset id %q", item.ID)
		}
		seen[item.ID] = true
		if len(item.AuthenticatedActions) == 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess %s requires authenticatedActions", item.ID)
		}
		for _, action := range item.AuthenticatedActions {
			if !allowed[action] {
				return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess authenticatedActions cannot include %q", action)
			}
		}
	}
	return nil
}
