package cli

import (
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func TestAllowedRepoReadFailsClosedWhenPolicyCannotBeRead(t *testing.T) {
	home := t.TempDir()
	// ReadAllow expects a JSON file here. A directory produces a stable read
	// error even when the test process has elevated filesystem permissions.
	if err := os.Mkdir(filepath.Join(home, "allow.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "alice"}
	if allowedRepoRead(home, flags, "kr://acme/private/core", "") {
		t.Fatal("an unreadable authorization policy must fail closed")
	}
}

func TestDatasetFileReadDoesNotImplyRepoKnowledge(t *testing.T) {
	rules := []AllowRule{{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.read", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("dataset file.read must not grant repository knowledge.read")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.search", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("dataset file.read must not grant repository knowledge.search")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "dataset.resolve",
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}); ok {
		t.Fatal("dataset file.read must not imply dataset.resolve")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "file.read",
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}); !ok {
		t.Fatal("file.read must still match itself on the dataset")
	}
}

func TestKnowledgeSetConsumeDoesNotImplyKnowledgeActions(t *testing.T) {
	rules := []AllowRule{{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.read", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("file.read must not grant knowledge.read")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.search",
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}); ok {
		t.Fatal("file.read must not grant knowledge.search")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "dataset.consume",
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}); ok {
		t.Fatal("dataset.consume is not a live alias")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "file.read",
		Catalog: "kr://acme/catalog", Dataset: "agent",
	}); !ok {
		t.Fatal("file.read must still match itself")
	}
}

func TestReadAllowRejectsRetiredKsetFieldAndActions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "allow.json")
	for _, body := range []string{
		`{"version":2,"rules":[{"id":"old","principal":"bot","actions":["file.read"],"catalog":"kr://acme/catalog","kset":"agent"}]}`,
		`{"version":2,"rules":[{"id":"old","principal":"bot","actions":["kset.consume"],"catalog":"kr://acme/catalog","dataset":"agent"}]}`,
	} {
		if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadAllow(home); err == nil {
			t.Fatalf("allow.json must reject retired kset aliases: %s", body)
		}
	}
}

func TestValidateActionsRejectsRetiredDatasetNames(t *testing.T) {
	for _, action := range []string{"kset.consume", "kset.manage", "kset.resolve", "dataset.consume"} {
		if err := validateActions([]string{action}); err == nil {
			t.Fatalf("%s must not be grantable", action)
		}
	}
	if err := validateActions([]string{"file.read", "dataset.resolve", "dataset.manage"}); err != nil {
		t.Fatal(err)
	}
}

func TestWriterPutPathHintIsStored(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		result := RunEmbeddedForTest(append([]string{"--home", dir}, args...), nil)
		if result.Status != 0 {
			t.Fatal(result.Stdout)
		}
	}
	core := "kr://acme/public/core"
	run("local", "init", "--catalog", "kr://acme/catalog")
	run("local", "repository", "attach", "--repo", core)
	run("attach", "--repo", core)
	run("writer", "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`)
	run("writer", "put", "--command-id", "listed", "--repo", core, "--object", "listed",
		"--schema-ref", "schema/policy.body", "--path-hint", "policies/listed.yaml",
		"--value", `{"body":"listed"}`)
	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	store, ok := ws.Store.Get(kernel.RepositoryID(core))
	if !ok {
		t.Fatal("missing repository")
	}
	head, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	krepo, err := ws.Reader.Require(kernel.RepositoryID(core), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	locator, ok := krepo.(knowledge.UnitLocator)
	if !ok {
		t.Fatalf("repository %T cannot locate unit paths", krepo)
	}
	paths, err := locator.ObjectUnitPaths("listed", head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "policies/listed.yaml" {
		t.Fatalf("writer put must store --path-hint: %q", paths)
	}
}

func TestAuthorizeKnowledgeSetKnowledgeSeparatesConsumeFromSearch(t *testing.T) {
	catalogID := "kr://acme/catalog"
	workspace := "agent"
	repo := "kr://acme/public/core"
	consume := AllowRule{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Catalog: catalogID, Dataset: workspace,
	}
	searchRepo := AllowRule{
		ID: "search", Principal: "bot", Actions: []string{"knowledge.search"}, Repo: repo,
	}
	catalogRead := AllowRule{
		ID: "cat", Principal: "bot", Actions: []string{"catalog.read"}, Catalog: catalogID,
	}
	searchQ := AllowQuery{
		Principal: "bot", Action: "knowledge.search", Catalog: catalogID, Dataset: workspace,
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, searchQ); err != nil {
		t.Fatalf("dataset file.read must admit pin knowledge.search: %v", err)
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{catalogRead, searchRepo}, searchQ); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("catalog.read must not skip named-dataset file.read: %v", err)
	}
	readQ := searchQ
	readQ.Action = "knowledge.read"
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, readQ); err != nil {
		t.Fatalf("dataset file.read admits pin knowledge.read: %v", err)
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, AllowQuery{
		Principal: "bot", Action: "knowledge.search", Repo: repo,
	}); err != errNotWorkspaceKnowledge {
		t.Fatalf("--repo SEARCH is not a named-dataset gate: %v", err)
	}
}

func TestDeliverSearchHitStripsSearchOnlyGrant(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{
		{ID: "s", Principal: "taihu:alice", Actions: []string{"knowledge.search"}, Repo: "kr://scene/knowledge"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "taihu:alice", "dataset": "scene-set", "catalog": "kr://scene/catalog"}
	hit := retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://scene/knowledge", Object: "metric/gmv"},
		Repository:   "kr://scene/knowledge",
		Address:      knowledge.Address{ObjectID: "metric/gmv"},
		Value:        map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := deliverSearchHit(home, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Knowledge.Value != nil {
		t.Fatalf("search-only must strip Canonical: %#v", out.Knowledge)
	}
}

func TestDeliverSearchHitKeepsDatasetFileReadBody(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{
		{ID: "c", Principal: "taihu:alice", Actions: []string{"file.read"}, Catalog: "kr://scene/catalog", Dataset: "scene-set"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "taihu:alice", "dataset": "scene-set", "catalog": "kr://scene/catalog"}
	hit := retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://scene/knowledge", Object: "metric/gmv"},
		Repository:   "kr://scene/knowledge",
		Address:      knowledge.Address{ObjectID: "metric/gmv"},
		Value:        map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := deliverSearchHit(home, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Knowledge.Value == nil {
		t.Fatal("dataset file.read must deliver listed-file bodies without repository knowledge.read")
	}
}

func TestDeliverSearchHitStripsInstanceMisTaggedAsSystemRepository(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{
		{ID: "s", Principal: "taihu:alice", Actions: []string{"knowledge.search"}, Repo: "kr://scene/knowledge"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "taihu:alice"}
	hit := retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: knowledge.SystemRepositoryID, Object: "metric/gmv"},
		Repository:   knowledge.SystemRepositoryID,
		Address:      knowledge.Address{ObjectID: "metric/gmv"},
		Value:        map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := deliverSearchHit(home, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Knowledge.Value != nil {
		t.Fatalf("undeclared repository must strip Canonical like any other repo: %#v", out.Knowledge)
	}
}

func TestDeliverSearchHitKeepsDeclaredRepositoryBody(t *testing.T) {
	home := t.TempDir()
	if err := WriteRepositoryAccess(home, RepositoryAccessFile{Repositories: []RepositoryAccess{SystemRepositoryAccess()}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "taihu:alice"}
	hit := retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: knowledge.SystemRepositoryID, Object: "schema/meta/schema-definition/v1"},
		Repository:   knowledge.SystemRepositoryID,
		Address:      knowledge.Address{ObjectID: "schema/meta/schema-definition/v1"},
		Value:        map[string]any{"title": "schema definition"},
	}}
	out, err := deliverSearchHit(home, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Knowledge.Value == nil {
		t.Fatal("authenticated default must deliver the declared repository body")
	}
}
