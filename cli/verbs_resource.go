package cli

import (
	apphome "kc/home"
	inv "kc/invocation"
	"kc/snapshot/dataset"
)

func resourceVerbs() map[string]command {
	return map[string]command{
		"knowledge-access": {Stage: stageGoverned, Run: inv.VerbResourceAccess},
		"knowledge-invoke": {Stage: stageGoverned, Run: inv.VerbResourceInvoke},
	}
}

// effectiveWorkspace is the recipe this command actually uses: registration state (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func effectiveWorkspace(ws *apphome.Home, home string, cat dataset.Service, setID string, flags map[string]FlagValue) (dataset.KnowledgeSet, error) {
	def, err := inv.EnsureWorkspace(ws, home, cat, setID)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	if ws.Deployment != nil {
		return def, nil
	}
	return inv.ApplyOverlay(home, flagString(flags, "as"), def)
}
