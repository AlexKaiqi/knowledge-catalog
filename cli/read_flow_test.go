package cli_test

import (
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
	body(t, kc(h, "append",
		"--command-id", "run-1",
		"--repo", core,
		"--stream", "runs",
		"--event-id", "evt-1",
		"--payload", `{"status":"ok"}`,
	))

	resolved := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "policy/A", "--commit", c2)))
	if resolved["status"] != "RESOLVED" || resolved["objectId"] != "policy/A" {
		t.Fatal(resolved)
	}
	if resolved["commit"] != c2 {
		t.Fatal("resolve must pin the requested commit", resolved["commit"])
	}
	viaRef := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if viaRef["status"] != "RESOLVED" {
		t.Fatal(viaRef)
	}
	missing := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "never/existed", "--ref", "refs/heads/main")))
	if missing["status"] != "UNRESOLVED" {
		t.Fatal(missing)
	}
	removed := asMap(t, body(t, kc(h, "resolve", "--repo", core, "--object", "policy/B", "--ref", "refs/heads/main")))
	if removed["status"] != "REMOVED" {
		t.Fatal(removed)
	}

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
	resolvedAspect := asMap(t, body(t, kc(h, "resolve",
		"--repo", core, "--object", "ETLTask:job-1", "--aspect", "io", "--ref", "refs/heads/main",
	)))
	if resolvedAspect["status"] != "RESOLVED" || asMap(t, resolvedAspect["address"])["aspectName"] != "io" || resolvedAspect["digest"] == "" {
		t.Fatal("resolve --aspect must return the exact unit Address and digest", resolvedAspect)
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

	listed := body(t, kc(h, "list", "--repo", core, "--ref", "refs/heads/main")).([]any)
	ids := map[string]bool{}
	for _, item := range listed {
		ids[asMap(t, asMap(t, item)["knowledgeRef"])["object"].(string)] = true
	}
	if !ids["policy/A"] || !ids["ETLTask:job-1"] || ids["policy/B"] {
		t.Fatal(ids)
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

	slice := asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "runs")))
	if asCursor(t, slice["cursor"]) == "" || slice["face"] != "continue" || asMap(t, slice["records"].([]any)[0])["eventId"] != "evt-1" {
		t.Fatal(slice)
	}
	page := asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "runs", "--limit", "1")))
	if page["face"] != "continue" || page["hasMore"] != false {
		t.Fatal(page)
	}
	hit := asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "runs", "--event-id", "evt-1")))
	if hit["face"] != "lookup" || asMap(t, hit["records"].([]any)[0])["eventId"] != "evt-1" {
		t.Fatal(hit)
	}
	empty := asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "missing")))
	if recs, ok := empty["records"].([]any); ok && len(recs) != 0 {
		t.Fatal(empty)
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
	expectMsg(t, kc(h, "stream", "--repo", core), "missing --stream")
	expectMsg(t, kc(h, "diff", "--repo", core, "--object", "policy/A", "--from", c1), "missing --to")
	expectCode(t, kc(h, "read", "--as", "other", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main"), "FORBIDDEN")
}
