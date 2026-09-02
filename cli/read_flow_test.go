package cli_test

import (
	"fmt"
	"testing"

	"kc/internal/testkit"
)

// TestCatalogRepoReadFlow extends the write loop through every Reader CLI verb.
// Workspace / read --workspace stay out; those are Catalog.
func TestCatalogRepoReadFlow(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))

	c1 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "a1",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--origin-kind", "SOURCE",
		"--source-ref", "handbook",
		"--actor-ref", "alice",
	)))["result"])["newCommit"].(string)
	c2 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "a2",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":2}`,
		"--origin-kind", "SOURCE",
		"--source-ref", "handbook",
	)))["result"])["newCommit"].(string)
	if c1 == c2 {
		t.Fatal("second put did not move commit")
	}
	body(t, kc(h, "put",
		"--command-id", "io",
		"--repo", core,
		"--object", "ETLTask:job-1",
		"--aspect", "io",
		"--value", `{"inputs":["a"]}`,
	))
	body(t, kc(h, "put",
		"--command-id", "own",
		"--repo", core,
		"--object", "ETLTask:job-1",
		"--aspect", "ownership",
		"--value", `{"owner":"alice"}`,
	))
	body(t, kc(h, "put",
		"--command-id", "b1",
		"--repo", core,
		"--object", "policy/B",
		"--value", `{"tmp":true}`,
	))
	body(t, kc(h, "remove", "--command-id", "drop-b", "--repo", core, "--object", "policy/B"))

	old := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--commit", c1)))
	if old["commit"] != c1 || asMap(t, old["value"])["v"] != float64(1) {
		t.Fatal("pinned commit followed latest", old)
	}
	live := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["v"] != float64(2) {
		t.Fatal(live)
	}
	aspect := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "ETLTask:job-1", "--aspect", "io", "--ref", "refs/heads/main")))
	if asMap(t, aspect["value"])["inputs"].([]any)[0] != "a" {
		t.Fatal(aspect)
	}
	if asMap(t, aspect["address"])["aspectName"] != "io" {
		t.Fatal(aspect["address"])
	}
	trimmed := asMap(t, body(t, kc(h, "read",
		"--repo", core, "--object", "ETLTask:job-1",
		"--exclude", "ownership", "--ref", "refs/heads/main",
	)))
	val := asMap(t, trimmed["value"])
	if val["io"] == nil {
		t.Fatal(trimmed["value"])
	}
	if _, ok := val["ownership"]; ok {
		t.Fatal("exclude ownership", trimmed["value"])
	}
	onlyIO := asMap(t, body(t, kc(h, "read",
		"--repo", core, "--object", "ETLTask:job-1",
		"--include", "io", "--ref", "refs/heads/main",
	)))
	onlyVal := asMap(t, onlyIO["value"])
	if onlyVal["io"] == nil || onlyVal["ownership"] != nil {
		t.Fatal(onlyIO["value"])
	}

	prov := asMap(t, body(t, kc(h, "provenance", "--repo", core, "--object", "policy/A", "--commit", c1)))
	if _, ok := prov["value"]; ok {
		t.Fatal("GET_PROVENANCE is not READ", prov)
	}
	chain := prov["chain"].([]any)
	if len(chain) == 0 || asMap(t, chain[0])["originKind"] != "SOURCE" {
		t.Fatal(prov)
	}
	if asMap(t, chain[0])["actorRef"] != "alice" {
		t.Fatal(chain[0])
	}

	history := body(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2)).([]any)
	if len(history) < 2 {
		t.Fatal(history)
	}
	if asMap(t, history[0])["commit"] != c2 {
		t.Fatal("LOG newest introducing commit first", history)
	}
	sawC1 := false
	for _, item := range history {
		if asMap(t, item)["commit"] == c1 {
			sawC1 = true
		}
	}
	if !sawC1 {
		t.Fatal(history)
	}

	delta := asMap(t, body(t, kc(h, "diff", "--repo", core, "--object", "policy/A", "--from", c1, "--to", c2)))
	if asMap(t, asMap(t, delta["from"])["value"])["v"] != float64(1) {
		t.Fatal(delta["from"])
	}
	if asMap(t, asMap(t, delta["to"])["value"])["v"] != float64(2) {
		t.Fatal(delta["to"])
	}

}

func TestCatalogRepoReadErrors(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"

	expectMsg(t, kc(h, "read", "--repo", core, "--object", "a", "--ref", "refs/heads/main"), "no kc home")

	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	c1 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "seed",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":1}`,
	)))["result"])["newCommit"].(string)

	expectMsg(t, kc(h, "read", "--object", "policy/A", "--ref", "refs/heads/main"), "missing --repo")
	expectMsg(t, kc(h, "read", "--repo", core, "--ref", "refs/heads/main"), "missing --object")
	expectMsg(t, kc(h, "read", "--repo", "kr://no/such", "--object", "policy/A", "--ref", "refs/heads/main"), "unknown repository")
	expectMsg(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/missing"), "does not exist")
	expectCode(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--commit", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"), "VERSION_UNRESOLVED")
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"), "VERSION_UNRESOLVED")
	expectCode(t, kc(h, "read", "--repo", core, "--object", "missing", "--commit", c1), "KNOWLEDGE_REF_UNRESOLVED")
	expectMsg(t, kc(h, "diff", "--repo", core, "--object", "policy/A", "--from", c1), "missing --to")
	expectCode(t, kc(h, "read", "--as", "other", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main"), "FORBIDDEN")
}

func TestAspectBindingResolveThroughCLIAndWorkspace(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	put := asMap(t, body(t, kc(h, "put", "--command-id", "binding-1", "--repo", core,
		"--object", "Service:orders", "--aspect", "health", "--value", "null",
		"--value-source", `{"kind":"binding","binding":{"mode":"state","runtime":"orders-runtime","protocol":"mcp","operations":{"read":{"call":"health.read"}}}}`)))
	commit := asMap(t, put["result"])["newCommit"].(string)
	resolved := asMap(t, body(t, kc(h, "resolve-binding", "--repo", core,
		"--object", "Service:orders", "--aspect", "health", "--commit", commit)))
	if resolved["mode"] != "state" || resolved["runtime"] != "orders-runtime" || resolved["declarationCommit"] != commit || resolved["declarationDigest"] == "" {
		t.Fatalf("pinned binding: %#v", resolved)
	}
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	workspace := body(t, kc(h, "resolve-binding", "--workspace", "agent", "--object", "Service:orders", "--aspect", "health")).([]any)
	if len(workspace) != 1 || asMap(t, workspace[0])["declarationCommit"] != commit {
		t.Fatalf("workspace binding: %#v", workspace)
	}
	expectCode(t, kc(h, "resource", "access", "--workspace", "agent", "--object", "Service:orders", "--aspect", "health"), "CAPABILITY_UNSATISFIED")
}

func TestWorkspaceSearchFailsClosedWhenAnyMemberCannotSatisfyQuery(t *testing.T) {
	h := testkit.TempDir(t)
	searchable := "kr://acme/public/searchable"
	opaque := "kr://acme/public/opaque"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", searchable))
	body(t, kc(h, "repo-add", "--repo", opaque))
	body(t, kc(h, "put", "--command-id", "schema", "--repo", searchable, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "hit", "--repo", searchable, "--object", "policy/A", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "put", "--command-id", "opaque", "--repo", opaque, "--object", "note/A", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", searchable+"=refs/heads/main", "--source", opaque+"=refs/heads/main"))
	syncIndexes(t, h, searchable)
	expectCode(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), "CAPABILITY_UNSATISFIED")
	expectMsg(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), opaque)
	expectMsg(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), "schema/*")
}

func TestWorkspaceSearchUnsatisfiedExplainsHowToRecover(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/public/opaque"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", repo))
	body(t, kc(h, "put", "--command-id", "opaque", "--repo", repo,
		"--object", "note/A", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", repo+"=refs/heads/main"))

	expectCode(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), "CAPABILITY_UNSATISFIED")
	expectMsg(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), "cannot satisfy SEARCH")
	expectMsg(t, kc(h, "search", "--workspace", "agent", "--query", "runbook"), "schema/*")
}

func TestWorkspaceSearchPublicContinuation(t *testing.T) {
	h := testkit.TempDir(t)
	one := "kr://acme/public/one"
	two := "kr://acme/public/two"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	values := [][]string{{"z", "a"}, {"y", "b"}}
	for i, repo := range []string{one, two} {
		body(t, kc(h, "repo-add", "--repo", repo))
		body(t, kc(h, "put", "--command-id", fmt.Sprintf("schema-%d", i), "--repo", repo, "--object", "schema/item.structure",
			"--value", `{"entity":"Item","pattern":"record","fields":{"name":{"type":"string","access":["filter","sort"]}}}`))
		for j, value := range values[i] {
			body(t, kc(h, "put", "--command-id", fmt.Sprintf("item-%d-%d", i, j), "--repo", repo, "--object", fmt.Sprintf("Item:%d:%d", i, j),
				"--value", fmt.Sprintf(`{"name":"%s"}`, value)))
		}
	}
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", one+"=refs/heads/main", "--source", two+"=refs/heads/main"))
	syncIndexes(t, h, one, two)
	first := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--exists", "name", "--sort", "name:desc", "--limit", "2")))
	continuation, _ := first["continuation"].(string)
	if got := workspaceSearchValues(t, first); fmt.Sprint(got) != "[z y]" || continuation == "" {
		t.Fatalf("first page: %#v", first)
	}
	second := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--exists", "name", "--sort", "name:desc", "--limit", "2", "--continuation", continuation)))
	if got := workspaceSearchValues(t, second); fmt.Sprint(got) != "[b a]" || second["continuation"] != nil {
		t.Fatalf("second page: %#v", second)
	}
	expectCode(t, kc(h, "search", "--workspace", "agent", "--prefix", "name=staging.", "--limit", "2", "--continuation", continuation), "PRECONDITION_FAILED")
}

func workspaceSearchValues(t *testing.T, result map[string]any) []string {
	t.Helper()
	values := []string{}
	for _, raw := range result["hits"].([]any) {
		hit := raw.(map[string]any)
		knowledge := hit["knowledge"].(map[string]any)
		value := knowledge["value"].(map[string]any)
		values = append(values, value["name"].(string))
	}
	return values
}
