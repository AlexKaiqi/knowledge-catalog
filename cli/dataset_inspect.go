package cli

import (
	"kc/catalog"
)

// filterCatalogState is the Catalog inventory view. Registration is visible to
// a principal who already passed catalog.read; knowledge.read still gates
// Canonical body, not whether the repository id appears here.
func filterCatalogState(_ string, _ map[string]FlagValue, state catalog.CatalogState) catalog.CatalogState {
	return catalog.NormalizeCatalogState(state)
}
