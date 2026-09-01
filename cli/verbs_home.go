package cli

import (
	"fmt"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// Local home verbs. These shape `.kc` — which Catalogs exist, which repositories
// are attached, which engines back them. `.kc` is where to find things on this
// machine; it is not a protocol object. The composed tree recipe is a
// Workspace (catalog.WorkspaceDefinition), not this home.

func homeVerbs() map[string]command {
	return map[string]command{
		"help":            {stage: stageHome, run: verbHelp},
		"init":            {stage: stageHome, run: verbInit},
		"store-ls":        {stage: stageHome, run: verbStoreLs},
		"bootstrap-grant": {stage: stageHome, run: verbBootstrapGrant},
		"audit":           {stage: stageHome, run: verbAudit},
		"catalog-add":     {stage: stageOpen, run: verbCatalogAdd},
		"store-set":       {stage: stageOpen, run: verbStoreSet},
		"repo-add":        {stage: stageOpen, run: verbRepoAdd},
		"status":          {stage: stageOpen, run: verbStatus},
	}
}

// verbBootstrapGrant is the one host-local authorization bootstrap. Product
// requests still cross the Server boundary with an explicit principal; this
// command only makes the first such principal an administrator of a new Home.
// It fails closed once any grant exists, so it cannot overwrite governance.
func verbBootstrapGrant(cx *invocation) (any, error) {
	if !homeReady(cx.Home) {
		return nil, missingHome(cx.Home)
	}
	principal, err := cx.require("principal")
	if err != nil {
		return nil, err
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	for range file.Rules {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed,
			"authorization is already initialized; manage grants through KC Server")
	}
	rule := AllowRule{ID: "bootstrap-local-admin", Principal: principal, Actions: []string{"*"}}
	file.Rules = append(file.Rules, rule)
	if err := WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return rule, nil
}

// verbHelp keeps help in the single transport command table. dispatch handles
// it before staging so an uninitialized home still works; the handler remains
// useful to table consumers and documents the command's result shape.
func verbHelp(cx *invocation) (any, error) {
	return helpFor(cx.flag("topic"))
}

func verbInit(cx *invocation) (any, error) {
	if cx.flag("namespace") != "" {
		return nil, fmt.Errorf("init takes --catalog <id> (kr://acme/catalog or acme/catalog), not --namespace")
	}
	catalogID := cx.flag("catalog")
	file, _, err := InitHome(cx.Home, catalogID)
	if err != nil {
		return nil, err
	}
	id := catalogID
	if id != "" {
		id, err = NormalizeCatalogID(id)
		if err != nil {
			return nil, err
		}
	} else if len(file.Catalogs) > 0 {
		id = file.Catalogs[0].ID
	}
	systemCommit, err := ensureSystemRepository(cx.Home, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"catalog": id, "system": systemRepositoryStatus(systemCommit)}, nil
}

func verbCatalogAdd(cx *invocation) (any, error) {
	catalogID, err := cx.require("catalog")
	if err != nil {
		return nil, err
	}
	stored, err := AddCatalog(cx.WS, catalogID)
	if err != nil {
		return nil, err
	}
	if repo, ok := cx.WS.Store.Get(knowledge.SystemRepositoryID); ok {
		cat, _, useErr := cx.WS.UseCatalog(stored)
		if useErr != nil {
			return nil, useErr
		}
		if err := cat.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
			return nil, err
		}
		head, err := repo.Head(snapshot.DefaultRef)
		if err != nil {
			return nil, err
		}
		return map[string]any{"catalog": stored, "system": systemRepositoryStatus(head)}, nil
	}
	return map[string]any{"catalog": stored}, nil
}

func verbStoreSet(cx *invocation) (any, error) {
	updated, err := applyStoreFlags(cx.WS.Stores, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := WriteStores(cx.Home, updated); err != nil {
		return nil, err
	}
	return PublicStores(updated), nil
}

func verbStoreLs(cx *invocation) (any, error) {
	if _, err := ReadHome(cx.Home); err != nil {
		return nil, err
	}
	file, err := ReadStores(cx.Home)
	if err != nil {
		return nil, err
	}
	return PublicStores(file), nil
}

func verbRepoAdd(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	head, err := AddRepository(cx.WS, repositoryID, cx.flag("driver"), cx.flag("dsn"), cx.flag("dir"), cx.flag("link"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"repositoryId": repositoryID, "head": head}, nil
}

// verbStatus mixes local machine facts (attached repositories, engines) with this
// Catalog's registry head. The protocol-level current state is
// `kc catalog show`; registry history is `kc catalog audit`.
func verbStatus(cx *invocation) (any, error) {
	if workspaceIDOf(cx.Flags) != "" || cx.flag("to") != "" {
		return statusMounts(cx)
	}
	ws := cx.WS
	cat, reg, err := ws.UseCatalog(cx.flag("catalog"))
	if err != nil {
		return nil, err
	}
	state := cat.DumpState()
	repos := make([]map[string]any, 0, len(ws.File.Repos))
	for _, r := range ws.File.Repos {
		item := map[string]any{"id": r.ID, "dir": r.Dir}
		if r.Driver != "" {
			item["driver"] = r.Driver
		}
		if r.DSN != "" {
			item["dsn"] = r.DSN
		}
		if repo, ok := ws.Store.Get(kernel.RepositoryID(r.ID)); ok {
			if head, err := repo.Head(defaultRef); err == nil {
				item["head"] = head
			}
			item["archived"] = repo.Archived()
		}
		repos = append(repos, item)
	}
	catalogs := make([]map[string]any, 0, len(ws.File.Catalogs))
	for _, item := range ws.File.Catalogs {
		row := map[string]any{"id": item.ID, "dir": item.Dir}
		if r := ws.Registries[item.ID]; r != nil {
			if head, err := r.Head(); err == nil {
				row["head"] = head
			}
		}
		catalogs = append(catalogs, row)
	}
	catalogHead, err := reg.Head()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"repos":    repos,
		"stores":   PublicStores(ws.Stores),
		"catalogs": catalogs,
		"catalog": map[string]any{
			"repositoryId": reg.CatalogID(),
			"head":         catalogHead,
		},
		"workspaces":   state.Workspaces,
		"repositories": state.Repositories,
		"archived":     state.Archived,
	}, nil
}

// verbAudit reads either the registry git history or the local process trail.
// It stays at stageHome so a workspace that will not mount can still be
// inspected, falling back to the local trail.
func verbAudit(cx *invocation) (any, error) {
	limit, err := limitFrom(cx.Flags, defaultAuditLimit)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = unboundedLimit
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
	ws, err := Open(cx.Home)
	if err != nil {
		entries, trailErr := readTrail(cx.Home, "", cmdFilter, limit)
		if trailErr != nil {
			return nil, err
		}
		return map[string]any{"source": "local", "entries": entries}, nil
	}
	defer ws.Close()
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
