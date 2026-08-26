package cli

import (
	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// inspectWorkspace composes Catalog state, the frozen Snapshot pin, logical
// AccessSpecs, and physical projection state at that pin.
func inspectWorkspace(ws *Home, flags map[string]FlagValue) (any, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(ws, ws.Dir, cat, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	pin := workspacePin(resolved)
	if err := requireCompleteWorkspaceRead(ws.Dir, flags, pin, ""); err != nil {
		return nil, err
	}
	plan, err := retrieval.PlanAccess(knowledge.Lookup(cat.Require), pin)
	if err != nil {
		return nil, err
	}
	indexes := []any{}
	for _, spec := range plan.Specs {
		repo, err := knowledge.Require(ws.Store, spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		desc, err := ws.Index.DescribeAt(repo, spec.Commit)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, desc)
	}
	return map[string]any{
		"catalog": filterCatalogState(ws.Dir, flags, cat.DumpState()),
		"pin":     resolved, "accessPlan": plan, "indexes": indexes,
	}, nil
}

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
