package cli

import (
	"kc/catalog"
)

func filterCatalogState(home string, flags map[string]FlagValue, state catalog.CatalogState) catalog.CatalogState {
	if ownerBypass(flags) {
		return catalog.NormalizeCatalogState(state)
	}
	visible := map[string]bool{}
	var repos []string
	for _, id := range state.Repositories {
		if allowedRepoRead(home, flags, id, "") {
			visible[id] = true
			repos = append(repos, id)
		}
	}
	var workspaces []catalog.WorkspaceDefinition
	for _, workspace := range state.Workspaces {
		var sources []catalog.WorkspaceSource
		for _, src := range workspace.Sources {
			if visible[string(src.Repository)] {
				sources = append(sources, src)
			}
		}
		if len(sources) == 0 {
			continue
		}
		copy := workspace
		copy.Sources = sources
		workspaces = append(workspaces, copy)
	}
	state.Repositories = repos
	state.Workspaces = workspaces
	return catalog.NormalizeCatalogState(state)
}
