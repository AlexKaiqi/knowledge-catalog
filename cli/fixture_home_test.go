package cli

import (
	"context"
	"fmt"
	"kc/internal/telemetry"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"strings"
)

// fixtureHomeOperations are Go test setup helpers, never product commands.
func fixtureHomeOperation(argv []string, runtime *telemetry.Runtime) (RunResult, bool) {
	parsed, err := ParseArgs(argv)
	if err != nil {
		return errorResult(err), true
	}
	operations := map[string]command{
		"local init":              {stage: stageHome, run: verbInit},
		"local status":            {stage: stageOpen, run: verbStatus},
		"local store show":        {stage: stageHome, run: verbStoreLs},
		"local store set":         {stage: stageOpen, run: verbStoreSet},
		"local grant bootstrap":   {stage: stageHome, run: verbBootstrapGrant},
		"local system publish":    {stage: stageHome, run: verbSystemPublish},
		"local catalog attach":    {stage: stageOpen, run: verbCatalogAdd},
		"local repository attach": {stage: stageOpen, run: verbRepoAdd},
		"local dataset overlay":   {stage: stageOpen, run: verbOverlay},
	}
	parts := append([]string{parsed.Command}, parsed.Args...)
	for n := len(parts); n > 0; n-- {
		path := strings.Join(parts[:n], " ")
		cmd, ok := operations[path]
		if !ok {
			continue
		}
		if len(parts[n:]) > 0 {
			if path == "local repository attach" && len(parts[n:]) == 1 && FlagString(parsed.Flags, "repo") == "" {
				parsed.Flags["repo"] = parts[n]
			} else {
				return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "unexpected fixture argument")), true
			}
		}
		action := strings.ReplaceAll(path, " ", ".")
		return invokeApplicationWithTelemetryAtHome(context.Background(), runtime, strings.ReplaceAll(path, " ", "-"), action, cmd, parsed.Flags, nil, nil), true
	}
	return RunResult{}, false
}

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
	systemCommit, err := EnsureSystemRepository(cx.Home, id)
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

func verbSystemPublish(cx *invocation) (any, error) {
	if !homeReady(cx.Home) {
		return nil, missingHome(cx.Home)
	}
	return PublishSystemRepository(cx.Home, cx.flag("driver"), cx.flag("dsn"), cx.flag("dir"))
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
		"datasets":     publicKnowledgeSets(state.KnowledgeSets),
		"repositories": state.Repositories,
		"archived":     state.Archived,
	}, nil
}

// verbAudit reads either the registry git history or the local process trail.
// It stays at stageHome so a workspace that will not mount can still be
// inspected, falling back to the local trail.
