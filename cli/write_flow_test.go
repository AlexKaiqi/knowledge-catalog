package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
)

// TestCatalogRepoWriteFlow is the closed loop up to write:
// init / catalog-add / repo-add / register / status, then every Writer verb
// (put, ingest, commit, receipt, remove, append) plus propose (candidate only).
// View / Release / merge are out of scope.
func TestCatalogRepoWriteFlow(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	docs := "kr://acme/docs/catalog"

	expectMsg(t, kc(h, "repo-add", "--repo", core), "no kc home")

	started := asMap(t, body(t, kc(h, "init", "--catalog", "acme/catalog")))
	if started["catalog"] != "kr://acme/catalog" {
		t.Fatal(started)
	}
	if _, ok := started["namespace"]; ok {
		t.Fatal("init must not echo namespace; the catalog id is enough", started)
	}
	if _, ok := started["home"]; ok {
		t.Fatal("init must not echo local --home", started)
	}
	if _, ok := started["created"]; ok {
		t.Fatal("init must not report created; access the catalog with read --catalog", started)
	}
	if _, ok := started["initialized"]; ok {
		t.Fatal("init must not report initialized", started)
	}
	again := asMap(t, body(t, kc(h, "init", "--catalog", "kr://acme/catalog")))
	if again["catalog"] != "kr://acme/catalog" {
		t.Fatal(again)
	}
	state := asMap(t, body(t, kc(h, "read", "--catalog")))
	if state["catalogId"] != "kr://acme/catalog" {
		t.Fatal(state)
	}
	if len(state["views"].([]any)) != 0 {
		t.Fatal(state)
	}
	if ids, _ := state["repositories"].([]any); len(ids) != 0 {
		t.Fatal("empty catalog has no registered repositories yet", state)
	}
	expectMsg(t, kc(h, "read"), "missing --repo")
	expectMsg(t, kc(h, "read-catalog"), "use: kc read --catalog")
	expectMsg(t, kc(h, "read-release"), "use: kc read --view")
	named := asMap(t, body(t, kc(h, "read", "--catalog", "kr://acme/catalog")))
	if named["catalogId"] != "kr://acme/catalog" {
		t.Fatal(named)
	}
	hist := asMap(t, body(t, kc(h, "audit")))
	if hist["catalogId"] != "kr://acme/catalog" {
		t.Fatal(hist)
	}
	sawInit := false
	for _, item := range hist["entries"].([]any) {
		msg, _ := asMap(t, item)["message"].(string)
		if len(msg) >= 4 && msg[:4] == "init" {
			sawInit = true
		}
	}
	if !sawInit {
		t.Fatal("catalog git history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog"), "kc audit")
	expectMsg(t, kc(h, "init", "--namespace", "acme"), "not --namespace")
	expectMsg(t, kc(h, "init", "--catalog", "acme"), "catalog id must be")
	expectMsg(t, kc(h, "init", "--catalog", "kr://other/catalog"), "already has catalog")

	empty := asMap(t, body(t, kc(h, "status")))
	if _, ok := empty["home"]; ok {
		t.Fatal("status must not echo local --home", empty)
	}
	if _, ok := empty["namespace"]; ok {
		t.Fatal("status must not echo namespace", empty)
	}
	if asMap(t, empty["catalog"])["repositoryId"] != "kr://acme/catalog" {
		t.Fatal(empty["catalog"])
	}
	if len(empty["repos"].([]any)) != 0 {
		t.Fatal(empty["repos"])
	}
	if ids, _ := empty["repositories"].([]any); len(ids) != 0 {
		t.Fatal(empty["repositories"])
	}

	addedCat := asMap(t, body(t, kc(h, "catalog-add", "--catalog", docs)))
	if addedCat["catalog"] != docs {
		t.Fatal(addedCat)
	}
	docsState := asMap(t, body(t, kc(h, "read", "--catalog", docs)))
	if docsState["catalogId"] != docs {
		t.Fatal(docsState)
	}
	expectMsg(t, kc(h, "catalog-add", "--catalog", docs), "already exists")

	mounted := asMap(t, body(t, kc(h, "repo-add", "--repo", core)))
	if mounted["repositoryId"] != core {
		t.Fatal(mounted)
	}
	head := mounted["head"].(string)
	if len(head) != 40 {
		t.Fatal(mounted["head"])
	}
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/catalog"), "reserved")
	expectMsg(t, kc(h, "repo-add", "--repo", docs), "reserved")

	st := asMap(t, body(t, kc(h, "status")))
	if len(st["catalogs"].([]any)) != 2 || len(st["repos"].([]any)) != 1 {
		t.Fatal(st["catalogs"], st["repos"])
	}
	if !hasRepository(st, core) {
		t.Fatal("repo-add should register into the default Catalog", st["repositories"])
	}
	docsStatus := asMap(t, body(t, kc(h, "status", "--catalog", docs)))
	if hasRepository(docsStatus, core) {
		t.Fatal("second Catalog must not inherit registered repositories", docsStatus["repositories"])
	}
	space := asMap(t, body(t, kc(h, "read", "--catalog")))
	if !hasRepository(space, core) {
		t.Fatal("read --catalog must list registered repositories", space)
	}
	docsSpace := asMap(t, body(t, kc(h, "read", "--catalog", docs)))
	if hasRepository(docsSpace, core) {
		t.Fatal("second Catalog dump must not inherit registered repositories", docsSpace)
	}

	registered := asMap(t, body(t, kc(h, "register", "--catalog", docs, "--repo", core)))
	if registered["catalog"] != docs || registered["repositoryId"] != core {
		t.Fatal(registered)
	}
	docsStatus = asMap(t, body(t, kc(h, "status", "--catalog", docs)))
	if !hasRepository(docsStatus, core) {
		t.Fatal(docsStatus["repositories"])
	}
	docsSpace = asMap(t, body(t, kc(h, "read", "--catalog", docs)))
	if !hasRepository(docsSpace, core) {
		t.Fatal(docsSpace)
	}

	body(t, kc(h, "put",
		"--command-id", "schema-policy",
		"--repo", core,
		"--object", "schema/policy",
		"--value", `{"entity":"Policy","aspect":"structure","pattern":"record"}`,
	))
	put := asMap(t, body(t, kc(h, "put",
		"--command-id", "seed-a",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--if-absent",
		"--schema-ref", "schema/policy",
		"--origin-kind", "SOURCE",
		"--source-ref", "handbook",
		"--actor-ref", "alice",
	)))
	if put["disposition"] != "APPLIED" || put["surface"] != "COMMIT" {
		t.Fatal(put)
	}
	mainAfterPut := asMap(t, put["result"])["newCommit"].(string)
	if mainAfterPut == head {
		t.Fatal("put did not move main")
	}
	replay := asMap(t, body(t, kc(h, "put",
		"--command-id", "seed-a",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--if-absent",
		"--schema-ref", "schema/policy",
		"--origin-kind", "SOURCE",
		"--source-ref", "handbook",
		"--actor-ref", "alice",
	)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatal(replay)
	}
	if asMap(t, replay["result"])["newCommit"] != mainAfterPut {
		t.Fatal("replay moved commit", replay["result"])
	}
	expectCode(t, kc(h, "put",
		"--command-id", "stale-cas",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"v":2}`,
		"--expected", head,
	), "NON_FAST_FORWARD")

	aspect := asMap(t, body(t, kc(h, "put",
		"--command-id", "seed-io",
		"--repo", core,
		"--object", "ETLTask:job-1",
		"--aspect", "io",
		"--value", `{"inputs":["a"]}`,
		"--origin-kind", "SOURCE",
	)))
	if aspect["disposition"] != "APPLIED" {
		t.Fatal(aspect)
	}
	body(t, kc(h, "put",
		"--command-id", "seed-b",
		"--repo", core,
		"--object", "policy/B",
		"--value", `{"tmp":true}`,
	))
	headBeforeIngest := statusRepo(t, asMap(t, body(t, kc(h, "status"))), core)["head"].(string)

	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := "---\nobject_id: runbooks/oncall\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(h, "cs.json")
	preview := asMap(t, body(t, kc(h, "ingest", "--repo", core, "--dir", draft, "--out", out)))
	if asMap(t, preview["files"].([]any)[0])["objectId"] != "runbooks/oncall" {
		t.Fatal(preview["files"])
	}
	headAfterIngest := statusRepo(t, asMap(t, body(t, kc(h, "status"))), core)["head"].(string)
	if headAfterIngest != headBeforeIngest {
		t.Fatal("ingest wrote to the repository")
	}
	committed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--changeset", out)))
	if committed["disposition"] != "APPLIED" {
		t.Fatal(committed)
	}
	receipt := asMap(t, body(t, kc(h, "receipt", "--command-id", "ingest-1")))
	if receipt["commandId"] != "ingest-1" || receipt["digest"] == "" {
		t.Fatal(receipt)
	}

	removed := asMap(t, body(t, kc(h, "remove",
		"--command-id", "drop-b",
		"--repo", core,
		"--object", "policy/B",
	)))
	if removed["disposition"] != "APPLIED" {
		t.Fatal(removed)
	}
	mainHead := asMap(t, removed["result"])["newCommit"].(string)

	appended := asMap(t, body(t, kc(h, "append",
		"--command-id", "run-1",
		"--repo", core,
		"--stream", "runs",
		"--event-id", "evt-1",
		"--payload", `{"status":"ok"}`,
	)))
	if appended["surface"] != "APPEND" || asCursor(t, asMap(t, appended["result"])["cursor"]) == "" {
		t.Fatal(appended)
	}
	afterAppend := asMap(t, body(t, kc(h, "status")))
	liveHead := statusRepo(t, afterAppend, core)["head"].(string)
	if liveHead != mainHead {
		t.Fatal("append moved git HEAD", liveHead, mainHead)
	}

	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/A", "--value", `{"v":99}`,
	)))
	if proposal["proposalId"] != "PR-1" || proposal["candidateCommit"] == "" {
		t.Fatal(proposal)
	}
	if proposal["candidateCommit"] == mainHead {
		t.Fatal("propose reused main commit", proposal)
	}
	afterPropose := asMap(t, body(t, kc(h, "status")))
	if statusRepo(t, afterPropose, core)["head"].(string) != mainHead {
		t.Fatal("propose moved main", afterPropose["repos"])
	}

	alive := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--commit", mainHead)))
	if asMap(t, alive["value"])["v"] != float64(1) {
		t.Fatal(alive)
	}
	candidate := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/candidates/PR-1")))
	if asMap(t, candidate["value"])["v"] != float64(99) {
		t.Fatal(candidate)
	}
	oncall := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "runbooks/oncall", "--ref", "refs/heads/main")))
	if asMap(t, oncall["value"])["text"] != "check freeze" {
		t.Fatal(oncall)
	}
	io := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "ETLTask:job-1", "--aspect", "io", "--ref", "refs/heads/main")))
	if asMap(t, io["value"])["inputs"].([]any)[0] != "a" {
		t.Fatal(io)
	}
	expectCode(t, kc(h, "read", "--repo", core, "--object", "policy/B", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
	slice := asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "runs")))
	if asCursor(t, slice["cursor"]) == "" || asMap(t, slice["records"].([]any)[0])["eventId"] != "evt-1" {
		t.Fatal(slice)
	}
}

func hasRepository(status map[string]any, repoID string) bool {
	raw, ok := status["repositories"].([]any)
	if !ok {
		return false
	}
	for _, item := range raw {
		if item == repoID {
			return true
		}
	}
	return false
}

func statusRepo(t *testing.T, status map[string]any, id string) map[string]any {
	t.Helper()
	repos, _ := status["repos"].([]any)
	for _, item := range repos {
		m := asMap(t, item)
		if m["id"] == id {
			return m
		}
	}
	t.Fatalf("missing repo %s in %#v", id, status["repos"])
	return nil
}

// TestCatalogRepoWriteErrors covers the same closed loop's failure modes.
// Protocol errors assert Code; CLI/workspace errors only have a message.
func TestCatalogRepoWriteErrors(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	docs := "kr://acme/docs/catalog"

	expectMsg(t, kc(h, "repo-add", "--repo", core), "no kc home")
	expectMsg(t, kc(h, "catalog-add", "--catalog", docs), "no kc home")
	expectMsg(t, kc(h, "status"), "no kc home")

	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "catalog-add", "--catalog", docs))
	expectMsg(t, kc(h, "catalog-add", "--catalog", docs), "already exists")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/catalog"), "reserved")
	expectMsg(t, kc(h, "repo-add", "--repo", docs), "reserved")
	expectMsg(t, kc(h, "register", "--repo", core), "unknown repository")
	expectMsg(t, kc(h, "status", "--catalog", "kr://missing/catalog"), "unknown catalog")
	expectMsg(t, kc(h, "register", "--catalog", "kr://missing/catalog", "--repo", core), "unknown catalog")

	body(t, kc(h, "repo-add", "--repo", core))
	expectMsg(t, kc(h, "repo-add", "--repo", core), "already registered")

	expectMsg(t, kc(h, "put", "--repo", core, "--object", "a", "--value", "1"), "missing --command-id")
	expectMsg(t, kc(h, "put", "--command-id", "x", "--object", "a", "--value", "1"), "missing --repo")
	expectMsg(t, kc(h, "put", "--command-id", "x", "--repo", core, "--object", "a"), "put requires --file or --value")
	expectMsg(t, kc(h, "put", "--command-id", "x", "--repo", "kr://no/such", "--object", "a", "--value", "1"), "unknown repository")
	expectMsg(t, kc(h, "receipt", "--command-id", "missing"), "unknown command-id")
	expectMsg(t, kc(h, "ingest", "--repo", core, "--dir", filepath.Join(h, "no-such-dir")), "no such file")

	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "a", "--value", `{"v":1}`))
	expectCode(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "a", "--value", `{"v":2}`), "IDEMPOTENCY_CONFLICT")
	expectCode(t, kc(h, "put", "--command-id", "absent", "--repo", core, "--object", "a", "--value", `{"v":3}`, "--if-absent"), "PRECONDITION_FAILED")
	expectCode(t, kc(h, "put",
		"--command-id", "derived-bad",
		"--repo", core,
		"--object", "derived/x",
		"--value", `{"v":1}`,
		"--origin-kind", "DERIVATION",
	), "PRECONDITION_FAILED")

	empty := filepath.Join(h, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"targetRepository":"kr://acme/public/core","targetRef":"refs/heads/main","operations":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kc(h, "commit", "--command-id", "empty", "--changeset", empty), "WRITE_TARGET_REQUIRED")

	stale := asCursor(t, asMap(t, body(t, kc(h, "stream", "--repo", core, "--stream", "runs")))["cursor"])
	body(t, kc(h, "append", "--command-id", "a1", "--repo", core, "--stream", "runs", "--event-id", "e1", "--payload", `{"n":1}`))
	expectCode(t, kc(h, "append", "--command-id", "stale", "--repo", core, "--stream", "runs", "--event-id", "e2", "--cursor", stale, "--payload", `{"n":2}`), "PRECONDITION_FAILED")
	expectCode(t, kc(h, "append", "--command-id", "conflict", "--repo", core, "--stream", "runs", "--event-id", "e1", "--payload", `{"n":9}`), "EVENT_ID_CONFLICT")

	h2 := testkit.TempDir(t)
	body(t, kc(h2, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h2, "repo-add", "--repo", core))
	expectCode(t, kc(h2, "put", "--as", "other", "--command-id", "y", "--repo", core, "--object", "a", "--value", "1"), "FORBIDDEN")

	body(t, kc(h, "archive-repo", "--repo", core))
	expectCode(t, kc(h, "put", "--command-id", "after-archive", "--repo", core, "--object", "z", "--value", `{"v":1}`), "REPOSITORY_ARCHIVED")
	expectCode(t, kc(h, "append", "--command-id", "after-archive-a", "--repo", core, "--stream", "runs", "--event-id", "e3", "--payload", `{"n":3}`), "REPOSITORY_ARCHIVED")
	expectCode(t, kc(h, "propose",
		"--proposal-id", "PR-arch", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-arch",
		"--object", "a", "--value", `{"v":9}`,
	), "REPOSITORY_ARCHIVED")
}

func TestSearchAfterPutIsIncremental(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	put1 := asMap(t, body(t, kc(h, "put", "--command-id", "i1", "--repo", core, "--object", "policy/A", "--value", `{"body":"needs a runbook"}`)))
	if put1["disposition"] != "APPLIED" {
		t.Fatal(put1)
	}
	hits1, ok := body(t, kc(h, "search", "--repo", core, "--query", "runbook")).([]any)
	if !ok || len(hits1) != 1 {
		t.Fatalf("%#v", hits1)
	}
	put2 := asMap(t, body(t, kc(h, "put", "--command-id", "i2", "--repo", core, "--object", "policy/B", "--value", `{"body":"second runbook"}`)))
	if put2["disposition"] != "APPLIED" {
		t.Fatal(put2)
	}
	hits2, ok := body(t, kc(h, "search", "--repo", core, "--query", "runbook")).([]any)
	if !ok || len(hits2) != 2 {
		t.Fatalf("expected 2 after second put, got %#v", hits2)
	}
	desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
	if desc["lagBehindHead"] != false || desc["objectCount"].(float64) != 2 {
		t.Fatalf("%#v", desc)
	}
	if desc["schemaDigest"] == "" {
		t.Fatal(desc)
	}
	lanes, _ := desc["lanes"].([]any)
	if len(lanes) == 0 {
		t.Fatalf("compiled spec missing: %#v", desc)
	}
}
