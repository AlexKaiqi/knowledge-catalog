package cli

import (
	"context"
	kchome "kc/home"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"strings"
	"sync"
)

var componentSystemFixture struct {
	sync.Mutex
	source *testkit.LakeFSFake
}

func componentSystemDSN() string {
	componentSystemFixture.Lock()
	defer componentSystemFixture.Unlock()
	if componentSystemFixture.source == nil {
		componentSystemFixture.source = testkit.StartLakeFSFake()
	}
	return componentSystemFixture.source.DSN(componentSystemFixture.source.NewRepo())
}

func CloseComponentFixturesForTest() {
	if componentSystemFixture.source != nil {
		componentSystemFixture.source.Close()
	}
}

// fixtureHomeOperations are Go test setup helpers, never product commands.
func fixtureHomeOperation(argv []string, runtime *telemetry.Runtime) (RunResult, bool) {
	parsed, err := ParseArgs(argv)
	if err != nil {
		return inv.ErrorResult(err), true
	}
	operations := map[string]inv.Command{
		"local init":              {Stage: invoke.StageHome, Run: verbInit},
		"local status":            {Stage: invoke.StageOpen, Run: verbStatus},
		"local store show":        {Stage: invoke.StageHome, Run: verbStoreLs},
		"local store set":         {Stage: invoke.StageOpen, Run: verbStoreSet},
		"local grant bootstrap":   {Stage: invoke.StageHome, Run: verbBootstrapGrant},
		"local system publish":    {Stage: invoke.StageHome, Run: verbSystemPublish},
		"local repository attach": {Stage: invoke.StageOpen, Run: verbRepoAdd},
		"local dataset overlay":   {Stage: invoke.StageOpen, Run: inv.VerbOverlay},
	}
	parts := append([]string{parsed.Command}, parsed.Args...)
	for n := len(parts); n > 0; n-- {
		path := strings.Join(parts[:n], " ")
		cmd, ok := operations[path]
		if !ok {
			continue
		}
		if len(parts[n:]) > 0 {
			if path == "local repository attach" && len(parts[n:]) == 1 && invoke.FlagString(parsed.Flags, "repo") == "" {
				parsed.Flags["repo"] = parts[n]
			} else {
				return inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "unexpected fixture argument")), true
			}
		}
		action := strings.ReplaceAll(path, " ", ".")
		return inv.InvokeApplicationWithTelemetryAtHome(context.Background(), runtime, strings.ReplaceAll(path, " ", "-"), action, cmd, parsed.Flags, nil, nil), true
	}
	return RunResult{}, false
}

func verbBootstrapGrant(cx *inv.Invocation) (any, error) {
	if !kchome.Ready(cx.Home) {
		return nil, kchome.Missing(cx.Home)
	}
	principal, err := cx.Require("principal")
	if err != nil {
		return nil, err
	}
	file, err := inv.ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	for range file.Rules {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed,
			"authorization is already initialized; manage grants through KC Server")
	}
	rule := inv.AllowRule{ID: "bootstrap-local-admin", Principal: principal, Actions: []string{"*"}}
	file.Rules = append(file.Rules, rule)
	if err := inv.WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return rule, nil
}

// verbHelp keeps help in the single transport command table. dispatch handles
// it before staging so an uninitialized home still works; the handler remains
// useful to table consumers and documents the command's result shape.

func verbInit(cx *inv.Invocation) (any, error) {
	if cx.Flag("catalog") != "" || cx.Flag("namespace") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "init has one registration space; catalog selection is retired")
	}
	file, _, err := kchome.InitHome(cx.Home, kchome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: componentSystemDSN()})
	if err != nil {
		return nil, err
	}
	bound := false
	for _, repo := range file.Repos {
		bound = bound || repo.ID == string(objectmodel.SystemRepositoryID)
	}
	if !bound {
		driver, dsn := cx.Flag("driver"), cx.Flag("dsn")
		if driver == "" && dsn == "" {
			driver, dsn = "lakefs", componentSystemDSN()
		}
		if _, err := kchome.PublishSystemRepository(cx.Home, driver, dsn, ""); err != nil {
			return nil, err
		}
		if err := ConfigureFixtureIndexSource(cx.Home, objectmodel.SystemRepositoryID, dsn); err != nil {
			return nil, err
		}
	}
	systemCommit, err := kchome.EnsureSystemRepository(cx.Home)
	if err != nil {
		return nil, err
	}
	return map[string]any{"system": inv.SystemRepositoryStatus(systemCommit)}, nil
}

func verbStoreSet(cx *inv.Invocation) (any, error) {
	updated, err := kchome.ApplyStoreFlags(cx.WS.Stores, fixtureStoreFlags(cx.Flags))
	if err != nil {
		return nil, err
	}
	if err := kchome.WriteStores(cx.Home, updated); err != nil {
		return nil, err
	}
	return kchome.PublicStores(updated), nil
}

func verbStoreLs(cx *inv.Invocation) (any, error) {
	if _, err := kchome.ReadHome(cx.Home); err != nil {
		return nil, err
	}
	file, err := kchome.ReadStores(cx.Home)
	if err != nil {
		return nil, err
	}
	return kchome.PublicStores(file), nil
}

func verbSystemPublish(cx *inv.Invocation) (any, error) {
	if !kchome.Ready(cx.Home) {
		return nil, kchome.Missing(cx.Home)
	}
	status, err := kchome.PublishSystemRepository(cx.Home, cx.Flag("driver"), cx.Flag("dsn"), cx.Flag("dir"))
	if err != nil {
		return nil, err
	}
	if status["seeded"] == true {
		dsn, _ := status["dsn"].(string)
		if err := ConfigureFixtureIndexSource(cx.Home, objectmodel.SystemRepositoryID, dsn); err != nil {
			return nil, err
		}
	}
	return status, nil
}

func verbRepoAdd(cx *inv.Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	head, err := kchome.AddRepository(cx.WS, repositoryID, cx.Flag("driver"), cx.Flag("dsn"), cx.Flag("dir"), cx.Flag("link"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"repositoryId": repositoryID, "head": head}, nil
}

// verbStatus mixes local machine facts (attached repositories, engines) with this
// Catalog's registry head. The protocol-level current state is
// `kc catalog show`; registry history is `kc catalog audit`.

func verbStatus(cx *inv.Invocation) (any, error) {
	ws := cx.WS
	state := ws.Datasets.DumpState()
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
			if head, err := repo.Head(snapshot.DefaultRef); err == nil {
				item["head"] = head
			}
			item["archived"] = repo.Archived()
		}
		repos = append(repos, item)
	}
	registryHead, err := ws.Registry.Head()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"repos":        repos,
		"stores":       kchome.PublicStores(ws.Stores),
		"registry":     map[string]any{"head": registryHead},
		"datasets":     inv.PublicKnowledgeSets(state.KnowledgeSets),
		"repositories": state.Repositories,
	}, nil
}

// verbAudit reads either the registry git history or the local process trail.
// It stays at stageHome so a workspace that will not mount can still be
// inspected, falling back to the local trail.

func fixtureStoreFlags(flags map[string]FlagValue) map[string]string {
	out := map[string]string{}
	for name := range flags {
		out[name] = invoke.FlagString(flags, name)
	}
	return out
}
