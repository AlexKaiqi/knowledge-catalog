package cli

import (
	"fmt"
	"kc/catalog"
)

func homeVerbs() map[string]command {
	return map[string]command{
		"help":                      {stage: stageHome, run: verbHelp},
		"catalog-audit":             {stage: stageHome, run: verbAudit},
		"deployment-init":           {stage: stageHome, run: verbClientOperation},
		"deployment-status":         {stage: stageHome, run: verbClientOperation},
		"deployment-system-publish": {stage: stageHome, run: verbClientOperation},
		"workspace-overlay":         {stage: stageHome, run: verbClientOperation},
	}
}

func verbHelp(cx *invocation) (any, error) {
	return helpFor(cx.flag("topic"))
}

func verbAudit(cx *invocation) (any, error) {
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return nil, err
	}
	layer := cx.flag("layer")
	cmdFilter := cx.flag("cmd")
	if layer != "" {
		if layer != "kc" && layer != "system" {
			return nil, fmt.Errorf("--layer must be kc or system")
		}
		entries, err := readTrail(cx.Home, layer, cmdFilter, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"source": "local", "layer": layer, "entries": entries}, nil
	}
	ws := cx.WS
	if ws == nil {
		var err error
		ws, err = Open(cx.Home)
		if err != nil {
			entries, trailErr := readTrail(cx.Home, "", cmdFilter, limit)
			if trailErr != nil {
				return nil, err
			}
			return map[string]any{"source": "local", "entries": entries}, nil
		}
		defer ws.Close()
	}
	cat, _, err := ws.UseCatalog(cx.flag("catalog"))
	if err != nil {
		return nil, err
	}
	hist := cat.Log(catalog.CatalogLogQuery{Limit: limit, Workspace: workspaceIDOf(cx.Flags)})
	return map[string]any{
		"source":    "catalog",
		"catalogId": hist.RepositoryID,
		"entries":   catalogLogEntries(hist.Commits, cmdFilter, limit),
	}, nil
}
