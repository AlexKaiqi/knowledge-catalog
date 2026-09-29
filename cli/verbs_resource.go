package cli

import (
	"kc/catalog"
	inv "kc/invocation"
)

func resourceVerbs() map[string]command {
	return map[string]command{
		"knowledge-access": {Stage: stageGoverned, Run: inv.VerbResourceAccess},
		"knowledge-invoke": {Stage: stageGoverned, Run: inv.VerbResourceInvoke},
	}
}

// effectiveWorkspace is the recipe this command actually uses: Catalog (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func effectiveWorkspace(ws *Home, home string, cat *catalog.Catalog, setID string, flags map[string]FlagValue) (catalog.KnowledgeSet, error) {
	def, err := inv.EnsureWorkspace(ws, home, cat, setID)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if ws.Deployment != nil {
		return def, nil
	}
	return inv.ApplyOverlay(home, flagString(flags, "as"), def)
}
