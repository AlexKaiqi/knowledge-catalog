package cli_test

import (
	"fmt"
	kchome "kc/home"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
)

// TestCatalogRepoReadFlow extends the write loop through every Reader CLI verb.
// Workspace / read --dataset stay out; those are Catalog.
func TestCatalogRepoReadFlow(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)

	c1 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "a1", "--value", `{"v":1}`, "--origin-kind", "SOURCE", "--source-ref", "handbook", "--actor-ref", "alice")))["result"])["newCommit"].(string)
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "a2", "--value", `{"v":2}`, "--origin-kind", "SOURCE", "--source-ref", "handbook")))["result"])["newCommit"].(string)
	if c1 == c2 {
		t.Fatal("second put did not move commit")
	}
	body(t, kc(h, "put", testObjectReference(t, core, "etl-task/job-1", "/io"), "--repo", core, "--command-id", "io", "--value", `{"inputs":["a"]}`))
	body(t, kc(h, "put", testObjectReference(t, core, "etl-task/job-1", "/ownership"), "--repo", core, "--command-id", "own", "--value", `{"owner":"alice"}`))
	body(t, kc(h, "put", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--command-id", "b1", "--value", `{"tmp":true}`))
	body(t, kc(h, "remove", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--command-id", "drop-b"))

	old := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--commit", c1)))
	if old["commit"] != c1 || asMap(t, old["value"])["v"] != float64(1) {
		t.Fatal("pinned commit followed latest", old)
	}
	live := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["v"] != float64(2) {
		t.Fatal(live)
	}
	aspect := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "etl-task/job-1", "/io"), "--repo", core, "--ref", "refs/heads/main")))
	if asMap(t, aspect["value"])["inputs"].([]any)[0] != "a" {
		t.Fatal(aspect)
	}
	if aspect["aspectName"] != "io" {
		t.Fatal(aspect)
	}
	selected := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "etl-task/job-1", "/io"), "--repo", core, "--ref", "refs/heads/main")))
	if selected["aspectName"] != "io" || len(asMap(t, selected["value"])) != 1 || asMap(t, selected["value"])["inputs"].([]any)[0] != "a" {
		t.Fatalf("locator must select only io, without the ownership Aspect: %#v", selected)
	}
	for _, flag := range []string{"--include", "--exclude"} {
		expectCode(t, kc(h, "read", testObjectReference(t, core, "etl-task/job-1", ""), "--repo", core, flag, "ownership"), "USAGE_INVALID")
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

	historyPage := asMap(t, body(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2)))
	history := historyPage["logs"].([]any)
	if historyPage["exhausted"] != nil || len(history) != 1 {
		t.Fatal(historyPage)
	}
	revisions := asMap(t, history[0])["revisions"].([]any)
	if len(revisions) < 2 {
		t.Fatal(historyPage)
	}
	if asMap(t, revisions[0])["commit"] != c2 {
		t.Fatal("LOG newest introducing commit first", historyPage)
	}
	sawC1 := false
	for _, item := range revisions {
		if asMap(t, item)["commit"] == c1 {
			sawC1 = true
		}
	}
	if !sawC1 {
		t.Fatal(historyPage)
	}

	firstPage := asMap(t, body(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--limit", "1")))
	if firstPage["exhausted"] != nil || firstPage["continuation"] == "" {
		t.Fatalf("object log must page with continuation: %#v", firstPage)
	}
	firstRevs := asMap(t, firstPage["logs"].([]any)[0])["revisions"].([]any)
	if len(firstRevs) != 1 || asMap(t, firstRevs[0])["commit"] != c2 {
		t.Fatalf("first log page: %#v", firstPage)
	}
	secondPage := asMap(t, body(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--limit", "1",
		"--continuation", firstPage["continuation"].(string))))
	secondRevs := asMap(t, secondPage["logs"].([]any)[0])["revisions"].([]any)
	if len(secondRevs) == 0 || asMap(t, secondRevs[0])["commit"] == c2 {
		t.Fatalf("continuation must resume after the first page: %#v", secondPage)
	}
	zeroPage := asMap(t, body(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--limit", "0")))
	if len(asMap(t, zeroPage["logs"].([]any)[0])["revisions"].([]any)) != len(revisions) {
		t.Fatalf("--limit 0 must mean the default history page: %#v", zeroPage)
	}
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--limit", "201"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--continuation", "not-a-cursor"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/B", "--commit", c2,
		"--continuation", firstPage["continuation"].(string)), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--member", "user:bob"), "USAGE_INVALID")
	expectCode(t, kc(h, "provenance", "--repo", core, "--object", "policy/A", "--commit", c2, "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "resolve", "--repo", core, "--object", "policy/A", "--member", "user:bob"), "USAGE_INVALID")

	resolved := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "policy/A", "--commit", c2)))
	if resolved["status"] != "RESOLVED" || resolved["commit"] != c2 {
		t.Fatalf("maintainer resolve: %#v", resolved)
	}
	aspectResolved := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "etl-task/job-1", "--aspect", "io")))
	if aspectResolved["status"] != "RESOLVED" || aspectResolved["aspectName"] != "io" {
		t.Fatalf("maintainer Address resolve: %#v", aspectResolved)
	}
	missingAspect := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "etl-task/job-1", "--aspect", "missing")))
	if missingAspect["status"] != "UNRESOLVED" {
		t.Fatalf("missing Address resolve must be UNRESOLVED: %#v", missingAspect)
	}
	missing := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "missing/nope", "--commit", c2)))
	if missing["status"] != "UNRESOLVED" {
		t.Fatalf("missing object resolve must be UNRESOLVED, not an empty READ: %#v", missing)
	}
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", c2, "--aspect", "io"), "USAGE_INVALID")

	ws, err := kchome.Open(h)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	delta, err := ws.Reader.Diff(kernel.RepositoryID(core), objectmodel.ObjectID("policy/A"), kernel.CommitID(c1), kernel.CommitID(c2))
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, delta.From.Value)["v"] != float64(1) {
		t.Fatal(delta.From)
	}
	if asMap(t, delta.To.Value)["v"] != float64(2) {
		t.Fatal(delta.To)
	}

}

func TestCatalogRepoReadErrors(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"

	expectMsg(t, kc(h, "read", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--ref", "refs/heads/main"), "no component fixture")

	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	c1 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`)))["result"])["newCommit"].(string)

	expectMsg(t, kc(h, "read", "--ref", "refs/heads/main"), "object URN")
	expectMsg(t, kc(h, "read", "--repo", core, "--ref", "refs/heads/main"), "object URN")
	expectMsg(t, kc(h, "read", testObjectReference(t, "kr://no/such", "policy/A", ""), "--repo", "kr://no/such", "--ref", "refs/heads/main"), "unknown repository")
	expectMsg(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--ref", "refs/heads/missing"), "does not exist")
	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--commit", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"), "VERSION_UNRESOLVED")
	expectCode(t, kc(h, "log", "--repo", core, "--object", "policy/A", "--commit", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"), "VERSION_UNRESOLVED")
	expectCode(t, kc(h, "read", testObjectReference(t, core, "fixture/missing", ""), "--repo", core, "--commit", c1), "OBJECT_REF_UNRESOLVED")
	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--as", "other", "--ref", "refs/heads/main"), "FORBIDDEN")
}

func TestAspectBindingResolveThroughCLIAndWorkspace(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	adoptLiveAccessFixture(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "schema/service.health", ""), "--repo", core, "--command-id", "binding-schema", "--value", `{"entity":"Service","aspect":"health","fields":{"status":{"type":"string"}}}`))
	body(t, kc(h, "put", testObjectReference(t, core, "service/orders", "/properties"), "--repo", core, "--command-id", "binding-entity", "--value", `{"name":"orders"}`))
	descriptor := putStateBinding(t, h, core, "service/orders", "health", "schema/service.health", "http://127.0.0.1:9")
	commit := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "service/orders", "/properties"), "--repo", core)))["commit"].(string)
	resolved := asMap(t, body(t, kc(h, "resolve-binding", "--repo", core,
		"--object", "service/orders", "--aspect", "health", "--commit", commit)))
	if resolved["mode"] != "state" || resolved["origin"] != "http://127.0.0.1:9" || resolved["schemaRef"] != "schema/service.health" || resolved["declarationCommit"] != commit || resolved["declarationDigest"] == "" ||
		resolved["descriptorRef"] != descriptor || resolved["descriptorCommit"] != commit || resolved["descriptorDigest"] == "" {
		t.Fatalf("pinned binding: %#v", resolved)
	}
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	workspace := body(t, kc(h, "resolve-binding", "--dataset", "agent", "--object", "service/orders", "--aspect", "health")).([]any)
	if len(workspace) != 1 || asMap(t, workspace[0])["declarationCommit"] != commit {
		t.Fatalf("workspace binding: %#v", workspace)
	}
	expectCode(t, kc(h, "access", "--dataset", "agent", "--object", "service/orders", "--aspect", "health"), "TEMPORARY_UNAVAILABLE")
}

func TestWorkspaceSearchFailsClosedWhenAnyMemberCannotSatisfyQuery(t *testing.T) {
	h := testkit.TempDir(t)
	searchable := "kr://acme/public/searchable"
	opaque := "kr://acme/public/opaque"
	body(t, kc(h, "init"))
	seedRepo(t, h, searchable)
	seedRepo(t, h, opaque)
	body(t, kc(h, "put", testObjectReference(t, searchable, "schema/policy.body", ""), "--repo", searchable, "--command-id", "schema", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", testObjectReference(t, searchable, "policy/A", ""), "--repo", searchable, "--command-id", "hit", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "put", testObjectReference(t, opaque, "note/A", ""), "--repo", opaque, "--command-id", "opaque", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", searchable+"=refs/heads/main", "--source", opaque+"=refs/heads/main"))
	syncIndexes(t, h, searchable)
	expectCode(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), "CAPABILITY_UNSATISFIED")
	expectMsg(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), opaque)
	expectMsg(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), "schema/*")
}

func TestWorkspaceSearchUnsatisfiedExplainsHowToRecover(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/public/opaque"
	body(t, kc(h, "init"))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "note/A", ""), "--repo", repo, "--command-id", "opaque", "--value", `{"body":"runbook"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repo+"=refs/heads/main"))

	expectCode(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), "CAPABILITY_UNSATISFIED")
	expectMsg(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), "cannot satisfy SEARCH")
	expectMsg(t, kc(h, "search", "--dataset", "agent", "--query", "runbook"), "schema/*")
}

func TestWorkspaceSearchPublicContinuation(t *testing.T) {
	h := testkit.TempDir(t)
	one := "kr://acme/public/one"
	two := "kr://acme/public/two"
	body(t, kc(h, "init"))
	values := [][]string{{"z", "a"}, {"y", "b"}}
	for i, repo := range []string{one, two} {
		seedRepo(t, h, repo)
		body(t, kc(h, "put", testObjectReference(t, repo, "schema/item.structure", ""), "--repo", repo, "--command-id", fmt.Sprintf("schema-%d", i), "--value", `{"entity":"Item","pattern":"record","fields":{"name":{"type":"string","access":["filter","sort"]}}}`))
		for j, value := range values[i] {
			body(t, kc(h, "put", testObjectReference(t, repo, fmt.Sprintf("item/%d-%d", i, j), ""), "--repo", repo, "--command-id", fmt.Sprintf("item-%d-%d", i, j), "--value", fmt.Sprintf(`{"name":"%s"}`, value)))
		}
	}
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", one+"=refs/heads/main", "--source", two+"=refs/heads/main"))
	syncIndexes(t, h, one, two)
	first := asMap(t, body(t, kc(h, "search", "--dataset", "agent", "--exists", "name", "--sort", "name:desc", "--limit", "2")))
	continuation, _ := first["continuation"].(string)
	if got := workspaceSearchValues(t, first); fmt.Sprint(got) != "[item/0-0 item/1-0]" || continuation == "" {
		t.Fatalf("first page: %#v", first)
	}
	second := asMap(t, body(t, kc(h, "search", "--dataset", "agent", "--exists", "name", "--sort", "name:desc", "--limit", "2", "--continuation", continuation)))
	if got := workspaceSearchValues(t, second); fmt.Sprint(got) != "[item/1-1 item/0-1]" || second["continuation"] != nil {
		t.Fatalf("second page: %#v", second)
	}
	expectCode(t, kc(h, "search", "--dataset", "agent", "--prefix", "name=staging.", "--limit", "2", "--continuation", continuation), "PRECONDITION_FAILED")
}

func workspaceSearchValues(t *testing.T, result map[string]any) []string {
	t.Helper()
	values := []string{}
	for _, raw := range result["hits"].([]any) {
		hit := raw.(map[string]any)
		values = append(values, fmt.Sprint(hit["objectId"]))
	}
	return values
}
