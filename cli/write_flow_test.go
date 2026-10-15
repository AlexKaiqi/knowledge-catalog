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
// Workspace composition and governed merge are exercised in their own journeys.
func TestCatalogRepoWriteFlow(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	expectMsg(t, kc(h, "repo-add", "--repo", core), "no component fixture")
	started := asMap(t, body(t, kc(h, "init")))
	for _, retired := range []string{"catalog", "namespace", "home", "created", "initialized"} {
		if _, present := started[retired]; present {
			t.Fatalf("init exposes obsolete or local context %s: %#v", retired, started)
		}
	}
	body(t, kc(h, "init"))
	state := asMap(t, body(t, kc(h, "show")))
	if _, present := state["catalogId"]; present || len(state["datasets"].([]any)) != 0 {
		t.Fatalf("initial inventory: %#v", state)
	}
	if ids := businessRepositories(state); len(ids) != 0 || !hasRepository(state, "kr://kc/system") {
		t.Fatal("new registration space must contain only the System Repository", state)
	}
	expectMsg(t, kc(h, "read"), "object URN")
	expectMsg(t, kc(h, "read-catalog"), "unknown command read-catalog")
	expectMsg(t, kc(h, "read-release"), "unknown command read-release")
	hist := asMap(t, body(t, kc(h, "registry", "audit")))
	if !catalogLogHas(t, hist, "init") {
		t.Fatalf("registry history must record initialization: %#v", hist)
	}
	expectCode(t, kc(h, "log", "--catalog", "old"), "USAGE_INVALID")
	expectMsg(t, kc(h, "writer", "put", testObjectReference(t, core, "note/x", ""), "--dataset", "agent", "--command-id", "x", "--value", `{}`), "rejects --dataset")
	expectCode(t, kc(h, "init", "--namespace", "acme"), "USAGE_INVALID")
	expectCode(t, kc(h, "catalog-add"), "USAGE_INVALID")
	empty := asMap(t, body(t, kc(h, "status")))
	if asMap(t, empty["registry"])["head"] == "" {
		t.Fatal("registry authority head unavailable", empty)
	}
	if len(empty["repos"].([]any)) != 1 || statusRepo(t, empty, "kr://kc/system")["driver"] != "lakefs" {
		t.Fatal(empty)
	}
	coreDSN := lakeFSRepoDSN(t)
	mounted := asMap(t, body(t, kc(h, "repo-add", "--repo", core, "--dsn", coreDSN)))
	if mounted["repositoryId"] != core {
		t.Fatal(mounted)
	}
	head := mounted["head"].(string)
	if head == "" {
		t.Fatal(mounted)
	}
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://kc/registry"), "reserved")
	st := asMap(t, body(t, kc(h, "status")))
	if len(st["repos"].([]any)) != 2 {
		t.Fatal(st)
	}
	if statusRepo(t, st, core)["dsn"] != coreDSN {
		t.Fatal("source authority was not retained", st)
	}
	if hasRepository(st, core) {
		t.Fatal("connection must not implicitly register a member", st)
	}
	registered := asMap(t, body(t, kc(h, "register", "--repo", core)))
	if registered["repositoryId"] != core {
		t.Fatal(registered)
	}
	if !hasRepository(asMap(t, body(t, kc(h, "show"))), core) {
		t.Fatal("explicit attach must expose source in inventory")
	}

	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy", ""), "--repo", core, "--command-id", "schema-policy", "--value", `{"entity":"Policy","pattern":"record"}`))
	put := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "seed-a", "--value", `{"v":1}`, "--if-absent", "--schema-ref", "schema/policy", "--origin-kind", "SOURCE", "--source-ref", "handbook", "--actor-ref", "alice")))
	if put["disposition"] != "APPLIED" || put["surface"] != "COMMIT" {
		t.Fatal(put)
	}
	mainAfterPut := asMap(t, put["result"])["newCommit"].(string)
	if mainAfterPut == head {
		t.Fatal("put did not move main")
	}
	replay := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "seed-a", "--value", `{"v":1}`, "--if-absent", "--schema-ref", "schema/policy", "--origin-kind", "SOURCE", "--source-ref", "handbook", "--actor-ref", "alice")))
	if replay["disposition"] != "REPLAYED" {
		t.Fatal(replay)
	}
	if asMap(t, replay["result"])["newCommit"] != mainAfterPut {
		t.Fatal("replay moved commit", replay["result"])
	}
	expectCode(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "stale-cas", "--value", `{"v":2}`, "--expected", head), "NON_FAST_FORWARD")

	aspect := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "etl-task/job-1", "/io"), "--repo", core, "--command-id", "seed-io", "--value", `{"inputs":["a"]}`, "--origin-kind", "SOURCE")))
	if aspect["disposition"] != "APPLIED" {
		t.Fatal(aspect)
	}
	body(t, kc(h, "put", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--command-id", "seed-b", "--value", `{"tmp":true}`))
	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := "---\nobject_id: runbooks/oncall\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(draft, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draft, "notes", "plain.md"), []byte("path-derived draft"), 0o644); err != nil {
		t.Fatal(err)
	}
	committed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--repo", core, "--dir", draft)))
	if committed["disposition"] != "APPLIED" {
		t.Fatal(committed)
	}
	replayed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--repo", core, "--dir", draft)))
	if replayed["disposition"] != "REPLAYED" {
		t.Fatal(replayed)
	}
	expectCode(t, kc(h, "commit", "--command-id", "ingest-unchanged", "--repo", core, "--dir", draft), "USAGE_INVALID")
	receipt := asMap(t, body(t, kc(h, "receipt", "--command-id", "ingest-1")))
	if receipt["commandId"] != "ingest-1" || receipt["digest"] == "" {
		t.Fatal(receipt)
	}

	removed := asMap(t, body(t, kc(h, "remove", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--command-id", "drop-b")))
	if removed["disposition"] != "APPLIED" {
		t.Fatal(removed)
	}
	mainHead := asMap(t, removed["result"])["newCommit"].(string)

	branch := asMap(t, body(t, kc(h, "branch", "--repo", core, "--name", "review/PR-1")))
	if branch["commitId"] != mainHead {
		t.Fatal("a new branch must start at the published head", branch)
	}
	candidateCommit := putOnRef(t, h, core, "refs/heads/review/PR-1", "policy/A", "pr-1", `{"v":99}`)
	if candidateCommit == mainHead {
		t.Fatal("branch write reused main commit", candidateCommit)
	}
	afterBranchWrite := asMap(t, body(t, kc(h, "status")))
	if statusRepo(t, afterBranchWrite, core)["head"].(string) != mainHead {
		t.Fatal("branch write moved main", afterBranchWrite["repos"])
	}

	alive := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--commit", mainHead)))
	if asMap(t, alive["value"])["v"] != float64(1) {
		t.Fatal(alive)
	}
	candidate := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--ref", "refs/heads/review/PR-1")))
	if asMap(t, candidate["value"])["v"] != float64(99) {
		t.Fatal(candidate)
	}
	oncall := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "runbooks/oncall", ""), "--repo", core, "--ref", "refs/heads/main")))
	if asMap(t, oncall["value"])["text"] != "check freeze" {
		t.Fatal(oncall)
	}
	io := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "etl-task/job-1", "/io"), "--repo", core, "--ref", "refs/heads/main")))
	if asMap(t, io["value"])["inputs"].([]any)[0] != "a" {
		t.Fatal(io)
	}
	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--ref", "refs/heads/main"), "OBJECT_REF_UNRESOLVED")
}

func hasRepository(status map[string]any, repoID string) bool {
	raw, ok := status["repositories"].([]any)
	if !ok {
		return false
	}
	for _, item := range raw {
		if repositoryInventoryID(item) == repoID {
			return true
		}
	}
	return false
}

func businessRepositories(status map[string]any) []any {
	raw, _ := status["repositories"].([]any)
	out := make([]any, 0, len(raw))
	for _, item := range raw {
		id := repositoryInventoryID(item)
		if id != "" && id != "kr://kc/system" {
			out = append(out, id)
		}
	}
	return out
}

func repositoryInventoryID(item any) string {
	switch value := item.(type) {
	case string:
		return value
	case map[string]any:
		id, _ := value["id"].(string)
		return id
	}
	return ""
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
	expectMsg(t, kc(h, "repo-add", "--repo", core), "no component fixture")
	expectCode(t, kc(h, "catalog-add"), "USAGE_INVALID")
	expectMsg(t, kc(h, "status"), "no component fixture")
	body(t, kc(h, "init"))
	expectCode(t, kc(h, "catalog", "use", "old"), "USAGE_INVALID")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://kc/registry"), "reserved")
	missingSnapshot := kc(h, "attach", "--repo", core)
	expectCode(t, missingSnapshot, "PRECONDITION_FAILED")
	expectMsg(t, missingSnapshot, "Snapshot "+core+" is unavailable")

	coreDSN := lakeFSRepoDSN(t)
	body(t, kc(h, "repo-add", "--repo", core, "--dsn", coreDSN))
	expectMsg(t, kc(h, "repo-add", "--repo", core, "--dsn", coreDSN), "already attached")
	body(t, kc(h, "register", "--repo", core))
	body(t, kc(h, "register", "--repo", core))

	expectMsg(t, kc(h, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--value", "1"), "missing --command-id")
	expectMsg(t, kc(h, "put", "--command-id", "x", "--value", "1"), "object URN")
	expectMsg(t, kc(h, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--command-id", "x"), "put requires --file or --value")
	expectMsg(t, kc(h, "put", testObjectReference(t, "kr://no/such", "fixture/a", ""), "--repo", "kr://no/such", "--command-id", "x", "--value", "1"), "unknown repository")
	expectMsg(t, kc(h, "receipt", "--command-id", "missing"), "unknown command-id")
	expectMsg(t, kc(h, "commit", "--command-id", "missing-dir", "--repo", core, "--dir", filepath.Join(h, "no-such-dir")), "no such file")

	body(t, kc(h, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	expectCode(t, kc(h, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":2}`), "IDEMPOTENCY_CONFLICT")
	expectCode(t, kc(h, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--command-id", "absent", "--value", `{"v":3}`, "--if-absent"), "PRECONDITION_FAILED")
	expectCode(t, kc(h, "put", testObjectReference(t, core, "derived/x", ""), "--repo", core, "--command-id", "derived-bad", "--value", `{"v":1}`, "--origin-kind", "DERIVATION"), "PRECONDITION_FAILED")

	empty := filepath.Join(h, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"targetRepository":"kr://acme/public/core","targetRef":"refs/heads/main","operations":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kc(h, "commit", "--command-id", "empty", "--changeset", empty), "USAGE_INVALID")

	h2 := testkit.TempDir(t)
	body(t, kc(h2, "init"))
	seedRepo(t, h2, core)
	expectCode(t, kc(h2, "put", testObjectReference(t, core, "fixture/a", ""), "--repo", core, "--as", "other", "--command-id", "y", "--value", "1"), "FORBIDDEN")

	body(t, kc(h, "detach", "--repo", core))
	body(t, kc(h, "put", testObjectReference(t, core, "fixture/z", ""), "--repo", core, "--command-id", "after-detach", "--value", `{"v":1}`))
	state := asMap(t, body(t, kc(h, "show")))
	if hasRepository(state, core) {
		t.Fatalf("detached repository must disappear from show: %#v", state)
	}
}

func TestSearchAfterPutIsIncremental(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy.body", ""), "--repo", core, "--command-id", "schema-body", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	put1 := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "i1", "--value", `{"body":"needs a runbook"}`)))
	if put1["disposition"] != "APPLIED" {
		t.Fatal(put1)
	}
	syncIndexes(t, h, core)
	search1 := asMap(t, body(t, kc(h, "search", "--repo", core, "--query", "runbook")))
	if hits := search1["hits"].([]any); len(hits) != 1 {
		t.Fatalf("%#v", search1)
	}
	put2 := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/B", ""), "--repo", core, "--command-id", "i2", "--value", `{"body":"second runbook"}`)))
	if put2["disposition"] != "APPLIED" {
		t.Fatal(put2)
	}
	syncIndexes(t, h, core)
	search2 := asMap(t, body(t, kc(h, "search", "--repo", core, "--query", "runbook")))
	if hits := search2["hits"].([]any); len(hits) != 2 {
		t.Fatalf("expected 2 after second put, got %#v", search2)
	}
	desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
	if desc["lagBehindHead"] != false || desc["objectCount"].(float64) != 2 {
		t.Fatalf("%#v", desc)
	}
	if desc["accessDigest"] == "" || desc["physicalDigest"] == "" || desc["providerRevision"] == "" {
		t.Fatal(desc)
	}
	lanes, _ := desc["lanes"].([]any)
	if len(lanes) == 0 {
		t.Fatalf("compiled spec missing: %#v", desc)
	}
}
