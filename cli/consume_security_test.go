package cli

import (
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
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

func TestWorkspaceConsumeDoesNotImplyKnowledgeActions(t *testing.T) {
	rules := []AllowRule{{
		ID: "ws", Principal: "bot", Actions: []string{"workspace.consume"},
		Catalog: "kr://acme/catalog", Workspace: "agent",
	}}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.read", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("workspace.consume must not grant knowledge.read")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "knowledge.search",
		Catalog: "kr://acme/catalog", Workspace: "agent",
	}); ok {
		t.Fatal("workspace.consume must not grant knowledge.search")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "workspace.consume",
		Catalog: "kr://acme/catalog", Workspace: "agent",
	}); !ok {
		t.Fatal("workspace.consume must still match itself")
	}
}

func TestAuthorizeWorkspaceKnowledgeSeparatesConsumeFromSearch(t *testing.T) {
	catalogID := "kr://acme/catalog"
	workspace := "agent"
	repo := "kr://acme/public/core"
	consume := AllowRule{
		ID: "ws", Principal: "bot", Actions: []string{"workspace.consume"},
		Catalog: catalogID, Workspace: workspace,
	}
	searchRepo := AllowRule{
		ID: "search", Principal: "bot", Actions: []string{"knowledge.search"}, Repo: repo,
	}
	catalogRead := AllowRule{
		ID: "cat", Principal: "bot", Actions: []string{"catalog.read"}, Catalog: catalogID,
	}
	searchQ := AllowQuery{
		Principal: "bot", Action: "knowledge.search", Catalog: catalogID, Workspace: workspace,
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, searchQ); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("consume alone must not admit knowledge.search: %v", err)
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{catalogRead, searchRepo}, searchQ); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("catalog.read must not skip named-set consume: %v", err)
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume, searchRepo}, searchQ); err != nil {
		t.Fatalf("repo-scoped knowledge.search must admit the named-set SEARCH verb: %v", err)
	}
	readQ := searchQ
	readQ.Action = "knowledge.read"
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, readQ); err != nil {
		t.Fatalf("consume admits the named-set knowledge.read verb; member grants are checked later: %v", err)
	}
	if err := authorizeWorkspaceKnowledge([]AllowRule{consume}, AllowQuery{
		Principal: "bot", Action: "knowledge.search", Repo: repo,
	}); err != errNotWorkspaceKnowledge {
		t.Fatalf("--repo SEARCH is not a named-set gate: %v", err)
	}
}

func TestDeliverSearchHitStripsSearchOnlyGrant(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{
		{ID: "c", Principal: "taihu:alice", Actions: []string{"workspace.consume"}, Catalog: "kr://scene/catalog", Workspace: "scene-set"},
		{ID: "s", Principal: "taihu:alice", Actions: []string{"knowledge.search"}, Repo: "kr://scene/knowledge"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"as": "taihu:alice", "workspace": "scene-set", "catalog": "kr://scene/catalog"}
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
		t.Fatalf("system-repo bypass must not leak instance Canonical: %#v", out.Knowledge)
	}
}

func TestDeliverSearchHitKeepsSystemSchema(t *testing.T) {
	home := t.TempDir()
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
		t.Fatal("authenticated callers may read System Repository schemas")
	}
}
