package cli

import (
	"kc/catalog"
	inv "kc/invocation"
	"os"
	"strings"

	knowledgeserving "kc/knowledge/serving"
)

func resourceVerbs() map[string]command {
	return map[string]command{
		"knowledge-access": {Stage: stageGoverned, Run: inv.VerbResourceAccess},
		"knowledge-invoke": {Stage: stageGoverned, Run: inv.VerbResourceInvoke},
	}
}

func ResourceLookup(cx *invocation) (knowledgeserving.StateLookup, error) {
	return inv.ResourceLookup(cx)
}

func RemoteServerURL(flags map[string]FlagValue) string {
	if value := strings.TrimSpace(FlagString(flags, "server")); value != "" {
		return value
	}
	if server := strings.TrimSpace(os.Getenv("KC_SERVER_URL")); server != "" {
		return server
	}
	return inv.SavedClientServer()
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
	return inv.ApplyOverlay(home, FlagString(flags, "as"), def)
}
