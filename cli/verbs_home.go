package cli

import (
	"fmt"
	"kc/catalog"
)

func homeVerbs() map[string]command {
	return map[string]command{
		"help":                        {Stage: stageHome, Run: verbHelp},
		"catalog-audit":               {Stage: stageHome, Run: verbAudit},
		"deployment-init":             {Stage: stageHome, Run: verbClientOperation},
		"deployment-status":           {Stage: stageHome, Run: verbClientOperation},
		"deployment-system-publish":   {Stage: stageHome, Run: verbClientOperation},
		"deployment-identity-migrate": {Stage: stageHome, Run: verbClientOperation},
		"dataset-overlay":             {Stage: stageHome, Run: verbClientOperation},
	}
}

func verbHelp(cx *invocation) (any, error) {
	return helpFor(cx.Flag("topic"))
}

func verbAudit(cx *invocation) (any, error) {
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return nil, err
	}
	layer := cx.Flag("layer")
	cmdFilter := cx.Flag("cmd")
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
	cat, _, err := ws.UseCatalog(resolveCurrentCatalog(cx))
	if err != nil {
		return nil, err
	}
	hist := cat.Log(catalog.CatalogLogQuery{Limit: limit, Dataset: setIDOf(cx.Flags)})
	return map[string]any{
		"source":    "catalog",
		"catalogId": hist.RepositoryID,
		"entries":   catalogLogEntries(hist.Commits, cmdFilter, limit),
	}, nil
}
