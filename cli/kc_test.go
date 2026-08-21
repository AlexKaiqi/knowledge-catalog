package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func kc(home string, args ...string) cli.RunResult {
	all := append([]string{"--home", home}, args...)
	return cli.Run(all)
}

func body(t *testing.T, result cli.RunResult) any {
	t.Helper()
	if result.Status != 0 {
		t.Fatalf("status %d stdout %s", result.Status, result.Stdout)
	}
	var value any
	if err := json.Unmarshal([]byte(result.Stdout), &value); err != nil {
		t.Fatal(err, result.Stdout)
	}
	return value
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not object: %#v", value)
	}
	return m
}

func asCursor(t *testing.T, value any) string {
	t.Helper()
	s, ok := value.(string)
	if !ok {
		t.Fatalf("cursor %#v", value)
	}
	return s
}

func failError(t *testing.T, result cli.RunResult) map[string]any {
	t.Helper()
	if result.Status != 1 {
		t.Fatalf("want status 1, got %d stdout %s", result.Status, result.Stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatal(err, result.Stdout)
	}
	return asMap(t, payload["error"])
}

func expectCode(t *testing.T, result cli.RunResult, code string) {
	t.Helper()
	err := failError(t, result)
	if err["code"] != code {
		t.Fatalf("want error code %s, got %#v", code, err)
	}
}

func expectMsg(t *testing.T, result cli.RunResult, substr string) {
	t.Helper()
	err := failError(t, result)
	msg, _ := err["message"].(string)
	if !strings.Contains(msg, substr) {
		t.Fatalf("want message containing %q, got %#v", substr, err)
	}
}

func TestParseSkipsBareDashDash(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"--", "serve", "--home", "/tmp/kc-demo", "--listen", "127.0.0.1:7380"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "serve" {
		t.Fatal(parsed)
	}
	if cli.FlagString(parsed.Flags, "home") != "/tmp/kc-demo" {
		t.Fatal(parsed.Flags)
	}
}

func TestHelp(t *testing.T) {
	result := cli.Run([]string{"help"})
	if result.Status != 0 {
		t.Fatal(result)
	}
	want := cli.Help
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}
	if result.Stdout != want {
		t.Fatalf("help mismatch")
	}
	for _, needle := range []string{"kc put", "kc ingest", "kc receipt", "kc read --catalog", "Output: ProvenanceTrace", "kc validate", "kc log", "kc audit", "kc hook", "kc gate", "kc serve", "kc store-set", "layout.yaml", "KC_REDIS_PASSWORD", "StarRocks"} {
		if !strings.Contains(result.Stdout, needle) {
			t.Fatal(needle)
		}
	}
	if strings.Contains(result.Stdout, "alias of") || strings.Contains(result.Stdout, "kc read-release") || strings.Contains(result.Stdout, "kc read-catalog") {
		t.Fatal("help must not advertise command aliases")
	}
}

func TestWalkthrough(t *testing.T) {
	h := testkit.TempDir(t)
	if kc(h, "init").Status != 0 {
		t.Fatal("init")
	}
	added := asMap(t, body(t, kc(h, "repo-add", "--repo", "kr://acme/public/core")))
	if head, _ := added["head"].(string); len(head) != 40 {
		t.Fatal(added["head"])
	}
	put := asMap(t, body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "ETLTask:job-1",
		"--aspect", "io",
		"--value", `{"inputs":["a"],"outputs":["b"]}`,
		"--origin-kind", "SOURCE",
		"--source-ref", "csv://runs",
	)))
	if put["disposition"] != "APPLIED" {
		t.Fatal(put)
	}
	commit := asMap(t, put["result"])["newCommit"].(string)
	replay := asMap(t, body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "ETLTask:job-1",
		"--aspect", "io",
		"--value", `{"inputs":["a"],"outputs":["b"]}`,
		"--origin-kind", "SOURCE",
		"--source-ref", "csv://runs",
	)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatal(replay)
	}
	resolved := asMap(t, body(t, kc(h, "resolve", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)))
	if resolved["status"] != "RESOLVED" {
		t.Fatal(resolved)
	}
	read := asMap(t, body(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)))
	value := asMap(t, read["value"])
	io := asMap(t, value["io"])
	if io["inputs"].([]any)[0] != "a" {
		t.Fatal(read["value"])
	}
	prov := asMap(t, body(t, kc(h, "provenance", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)))
	if _, ok := prov["value"]; ok {
		t.Fatal(prov)
	}
	chain := prov["chain"].([]any)
	if asMap(t, chain[0])["originKind"] != "SOURCE" {
		t.Fatal(prov)
	}
	appended := asMap(t, body(t, kc(h, "append",
		"--command-id", "run-1",
		"--repo", "kr://acme/public/core",
		"--stream", "runs",
		"--event-id", "evt-1",
		"--payload", `{"status":"ok"}`,
	)))
	if asCursor(t, asMap(t, appended["result"])["cursor"]) == "" {
		t.Fatal(appended)
	}
	slice := asMap(t, body(t, kc(h, "stream", "--repo", "kr://acme/public/core", "--stream", "runs")))
	if asCursor(t, slice["cursor"]) != asCursor(t, asMap(t, appended["result"])["cursor"]) {
		t.Fatal(slice, appended)
	}
	if asMap(t, slice["records"].([]any)[0])["eventId"] != "evt-1" {
		t.Fatal(slice)
	}
	body(t, kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	later := filepath.Join(h, "later.json")
	if err := os.WriteFile(later, []byte(`{"inputs":["changed"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "put", "--command-id", "sync-2", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--aspect", "io", "--file", later))
	serving := body(t, kc(h, "read", "--view", "agent", "--object", "ETLTask:job-1")).([]any)
	if asMap(t, asMap(t, serving[0])["value"])["io"].(map[string]any)["inputs"].([]any)[0] != "changed" {
		t.Fatal(serving)
	}
	live := asMap(t, body(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--ref", "refs/heads/main")))
	if asMap(t, asMap(t, live["value"])["io"])["inputs"].([]any)[0] != "changed" {
		t.Fatal(live)
	}
}

func TestProtocolErrorJSON(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	expectCode(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "missing", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
}

func TestIdempotencyConflict(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"))
	expectCode(t, kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core", "--object", "a", "--value", "2"), "IDEMPOTENCY_CONFLICT")
}

func TestCatalogLogDiff(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	status := asMap(t, body(t, kc(h, "status")))
	if _, ok := status["namespace"]; ok {
		t.Fatal("status must not echo namespace", status["namespace"])
	}
	if asMap(t, status["catalog"])["repositoryId"] != "kr://acme/catalog" {
		t.Fatal(status["catalog"])
	}
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/catalog"), "reserved")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	first := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v1", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--value", `{"version":1}`)))["result"])
	second := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--value", `{"version":2}`)))["result"])
	history := body(t, kc(h, "log", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--commit", second["newCommit"].(string))).([]any)
	if asMap(t, history[0])["commit"] != second["newCommit"] {
		t.Fatal(history)
	}
	sawFirst := false
	for _, item := range history {
		if asMap(t, item)["commit"] == first["newCommit"] {
			sawFirst = true
		}
	}
	if !sawFirst {
		t.Fatal(history)
	}
	delta := asMap(t, body(t, kc(h, "diff", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--from", first["newCommit"].(string), "--to", second["newCommit"].(string))))
	if asMap(t, asMap(t, delta["from"])["value"])["version"] != float64(1) {
		t.Fatal(delta)
	}
	body(t, kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	catalogLog := asMap(t, body(t, kc(h, "audit", "--view", "agent")))
	sawDefine := false
	for _, item := range catalogLog["entries"].([]any) {
		msg := asMap(t, item)["message"].(string)
		if strings.HasPrefix(msg, "define-view") {
			sawDefine = true
		}
	}
	if !sawDefine {
		t.Fatal(catalogLog)
	}
}

func TestProposeMergeIsVisibleOnView(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "seed", "--repo", "kr://acme/public/core", "--object", "policy/P-103", "--value", `{"v":1}`))
	body(t, kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", "kr://acme/public/core",
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-103", "--value", `{"v":2}`,
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--view", "agent")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "S7", "--outcome", "PASSED")))
	merged := asMap(t, body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string))))
	if merged["commitId"] != proposal["candidateCommit"] {
		t.Fatal(merged, proposal)
	}
	serving := body(t, kc(h, "read", "--view", "agent", "--object", "policy/P-103")).([]any)
	if asMap(t, serving[0])["value"].(map[string]any)["v"] != float64(2) {
		t.Fatal(serving)
	}
}

func TestMultipleCatalogs(t *testing.T) {
	h := testkit.TempDir(t)
	started := asMap(t, body(t, kc(h, "init", "--catalog", "kr://acme/catalog")))
	if started["catalog"] != "kr://acme/catalog" {
		t.Fatal(started)
	}
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put",
		"--command-id", "seed",
		"--repo", "kr://acme/public/core",
		"--object", "policy/P-1",
		"--value", `{"v":1}`,
	))
	added := asMap(t, body(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs/catalog")))
	if added["catalog"] != "kr://acme/docs/catalog" {
		t.Fatal(added)
	}
	expectMsg(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs/catalog"), "already exists")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/docs/catalog"), "reserved")
	body(t, kc(h, "define-view", "--view", "ops", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	body(t, kc(h, "register", "--catalog", "kr://acme/docs/catalog", "--repo", "kr://acme/public/core"))
	body(t, kc(h, "define-view",
		"--catalog", "kr://acme/docs/catalog",
		"--view", "docs",
		"--revision", "1",
		"--source", "kr://acme/public/core=refs/heads/main",
	))
	status := asMap(t, body(t, kc(h, "status")))
	catalogs := status["catalogs"].([]any)
	if len(catalogs) != 2 {
		t.Fatal(status["catalogs"])
	}
	ids := map[string]bool{}
	for _, item := range catalogs {
		ids[asMap(t, item)["id"].(string)] = true
	}
	if !ids["kr://acme/catalog"] || !ids["kr://acme/docs/catalog"] {
		t.Fatal(status["catalogs"])
	}
	if _, ok := status["releases"]; ok {
		t.Fatal("status must not list releases", status["releases"])
	}
	other := asMap(t, body(t, kc(h, "status", "--catalog", "kr://acme/docs/catalog")))
	if asMap(t, other["catalog"])["repositoryId"] != "kr://acme/docs/catalog" {
		t.Fatal(other["catalog"])
	}
	sawDocs, sawOps := false, false
	for _, item := range other["views"].([]any) {
		switch asMap(t, item)["viewId"] {
		case "docs":
			sawDocs = true
		case "ops":
			sawOps = true
		}
	}
	if !sawDocs || sawOps {
		t.Fatal(other["views"])
	}
	serving := body(t, kc(h, "read", "--catalog", "kr://acme/docs/catalog", "--view", "docs", "--object", "policy/P-1")).([]any)
	if asMap(t, serving[0])["value"].(map[string]any)["v"] != float64(1) {
		t.Fatal(serving)
	}
	catalogLog := asMap(t, body(t, kc(h, "audit", "--catalog", "kr://acme/docs/catalog", "--view", "docs")))
	if len(catalogLog["entries"].([]any)) == 0 {
		t.Fatal(catalogLog)
	}
	expectMsg(t, kc(h, "define-view",
		"--catalog", "kr://missing/catalog",
		"--view", "x",
		"--revision", "1",
		"--source", "kr://acme/public/core=refs/heads/main",
	), "unknown catalog")
}

func TestLifecycleAndAllow(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put", "--command-id", "seed", "--repo", "kr://acme/public/core", "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-view", "--view", "ops", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	got := body(t, kc(h, "read", "--view", "ops", "--object", "policy/P-1")).([]any)
	if len(got) != 1 {
		t.Fatal(got)
	}
	body(t, kc(h, "retire-view", "--view", "ops"))
	expectCode(t, kc(h, "read", "--view", "ops", "--object", "policy/P-1"), "VIEW_GENERATION_INVALID")
	expectMsg(t, kc(h, "pin-view", "--view", "ops"), "unknown command pin-view")
	body(t, kc(h, "archive-catalog"))
	expectCode(t, kc(h, "define-view", "--view", "later", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"), "CATALOG_ARCHIVED")

	h2 := testkit.TempDir(t)
	kc(h2, "init", "--catalog", "kr://acme/catalog")
	kc(h2, "repo-add", "--repo", "kr://acme/public/core")
	rule := asMap(t, body(t, kc(h2, "allow", "--principal", "bot", "--cmd", "put,remove,commit", "--repo", "kr://acme/public/core")))
	if rule["id"] == "" {
		t.Fatal(rule)
	}
	if denied := kc(h2, "put", "--as", "bot", "--command-id", "x", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"); denied.Status != 0 {
		t.Fatal(denied)
	}
	expectCode(t, kc(h2, "put", "--as", "other", "--command-id", "y", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"), "FORBIDDEN")
	body(t, kc(h2, "archive-repo", "--repo", "kr://acme/public/core"))
	expectCode(t, kc(h2, "put", "--command-id", "z", "--repo", "kr://acme/public/core", "--object", "b", "--value", "2"), "REPOSITORY_ARCHIVED")
}

func TestCompanyCatalogDoesNotGrantByView(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/physical"
	fin := "kr://acme/groups/finance"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "repo-add", "--repo", pub)
	kc(h, "repo-add", "--repo", fin)
	body(t, kc(h, "put", "--command-id", "pub-1", "--repo", pub, "--object", "Table:orders", "--value", `{"src":"public"}`))
	body(t, kc(h, "put", "--command-id", "fin-1", "--repo", fin, "--object", "Table:orders", "--value", `{"src":"finance"}`))
	body(t, kc(h, "define-view", "--view", "company", "--revision", "1",
		"--source", pub+"=refs/heads/main",
		"--source", fin+"=refs/heads/main"))

	body(t, kc(h, "allow", "--principal", "qa-bot", "--cmd", "read", "--repo", pub))
	body(t, kc(h, "allow", "--principal", "qa-bot", "--cmd", "read-view", "--catalog", "kr://acme/catalog", "--view", "company"))
	body(t, kc(h, "allow", "--principal", "finance-bot", "--cmd", "read", "--repo", pub))
	body(t, kc(h, "allow", "--principal", "finance-bot", "--cmd", "read", "--repo", fin))
	body(t, kc(h, "allow", "--principal", "finance-bot", "--cmd", "read-view", "--catalog", "kr://acme/catalog", "--view", "company"))

	expectCode(t, kc(h, "read", "--as", "qa-bot", "--repo", fin, "--object", "Table:orders", "--ref", "refs/heads/main"), "FORBIDDEN")
	qaRead := asMap(t, body(t, kc(h, "read", "--as", "qa-bot", "--repo", pub, "--object", "Table:orders", "--ref", "refs/heads/main")))
	if asMap(t, qaRead["value"])["src"] != "public" {
		t.Fatal(qaRead)
	}

	qaRelease := body(t, kc(h, "read", "--as", "qa-bot", "--view", "company", "--object", "Table:orders")).([]any)
	if len(qaRelease) != 1 || asMap(t, qaRelease[0])["repository"] != pub {
		t.Fatalf("view must not grant finance: %#v", qaRelease)
	}
	if asMap(t, asMap(t, qaRelease[0])["value"])["src"] != "public" {
		t.Fatal(qaRelease)
	}

	finRelease := body(t, kc(h, "read", "--as", "finance-bot", "--view", "company", "--object", "Table:orders")).([]any)
	if len(finRelease) != 2 {
		t.Fatalf("finance-bot should see both members: %#v", finRelease)
	}
	seen := map[string]bool{}
	for _, item := range finRelease {
		seen[asMap(t, item)["repository"].(string)] = true
	}
	if !seen[pub] || !seen[fin] {
		t.Fatal(finRelease)
	}
}

func TestCatalogIsolationDoesNotShareAllow(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/physical"
	secret := "kr://acme/restricted/classif"
	iso := "kr://acme/restricted/catalog"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "catalog-add", "--catalog", iso)
	kc(h, "repo-add", "--repo", pub)
	kc(h, "repo-add", "--repo", secret)
	body(t, kc(h, "register", "--catalog", iso, "--repo", secret))
	body(t, kc(h, "put", "--command-id", "pub-1", "--repo", pub, "--object", "Table:orders", "--value", `{"src":"public"}`))
	body(t, kc(h, "put", "--command-id", "sec-1", "--repo", secret, "--object", "Table:orders", "--value", `{"src":"secret"}`))
	body(t, kc(h, "define-view", "--view", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "define-view", "--catalog", iso, "--view", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))

	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read", "--repo", pub))
	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read-view", "--catalog", "kr://acme/catalog", "--view", "company"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read", "--repo", secret))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read-view", "--catalog", iso, "--view", "classif"))

	crew := body(t, kc(h, "read", "--as", "crew-bot", "--view", "company", "--object", "Table:orders")).([]any)
	if len(crew) != 1 || asMap(t, crew[0])["repository"] != pub {
		t.Fatalf("%#v", crew)
	}
	expectCode(t, kc(h, "read", "--as", "crew-bot", "--catalog", iso, "--view", "classif", "--object", "Table:orders"), "FORBIDDEN")
	expectCode(t, kc(h, "read", "--as", "classif-bot", "--view", "company", "--object", "Table:orders"), "FORBIDDEN")
	classif := body(t, kc(h, "read", "--as", "classif-bot", "--catalog", iso, "--view", "classif", "--object", "Table:orders")).([]any)
	if len(classif) != 1 || asMap(t, classif[0])["repository"] != secret {
		t.Fatalf("%#v", classif)
	}
}

func TestForkPublishDoesNotCopyPersonal(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/semantic"
	alice := "kr://acme/personals/alice"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	kc(h, "repo-add", "--repo", pub)
	kc(h, "repo-add", "--repo", alice)
	draft := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "alice-draft",
		"--repo", alice,
		"--object", "drafts/metric-x",
		"--value", `{"text":"alice draft"}`,
	)))["result"])
	source := alice + "@" + draft["newCommit"].(string) + "/drafts/metric-x"
	body(t, kc(h, "define-view", "--view", "semantic", "--revision", "1", "--source", pub+"=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "FORK-1", "--repo", pub,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/FORK-1",
		"--object", "metrics/x", "--value", `{"text":"published"}`,
		"--origin-kind", "ASSERTION",
		"--source-ref", "kc://"+strings.TrimPrefix(source, "kr://"),
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "FORK-1", "--view", "semantic")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "fork", "--outcome", "PASSED")))
	body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string)))

	live := asMap(t, body(t, kc(h, "read", "--repo", pub, "--object", "metrics/x", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["text"] != "published" {
		t.Fatal(live)
	}
	personal := asMap(t, body(t, kc(h, "read", "--repo", alice, "--object", "drafts/metric-x", "--ref", "refs/heads/main")))
	if asMap(t, personal["value"])["text"] != "alice draft" {
		t.Fatal(personal)
	}
	expectCode(t, kc(h, "read", "--repo", pub, "--object", "drafts/metric-x", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
	prov := asMap(t, body(t, kc(h, "provenance", "--repo", pub, "--object", "metrics/x", "--ref", "refs/heads/main")))
	chain := prov["chain"].([]any)
	refs := asMap(t, chain[0])["sourceRefs"].([]any)
	if len(refs) != 1 || refs[0] != "kc://acme/personals/alice@"+draft["newCommit"].(string)+"/drafts/metric-x" {
		t.Fatal(prov)
	}
	servingNew := body(t, kc(h, "read", "--view", "semantic", "--object", "metrics/x")).([]any)
	if len(servingNew) != 1 {
		t.Fatal("merged fork must be visible on next read --view", servingNew)
	}
	servingDraft := body(t, kc(h, "read", "--view", "semantic", "--object", "drafts/metric-x")).([]any)
	if len(servingDraft) != 0 {
		t.Fatal("personal draft leaked into public view", servingDraft)
	}
}

func TestSchemaRefOnProposeAndAppend(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	kc(h, "repo-add", "--repo", core)
	expectCode(t, kc(h, "propose",
		"--proposal-id", "PR-schema",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	), "SCHEMA_REVISION_UNRESOLVED")
	expectCode(t, kc(h, "append",
		"--command-id", "run-bad",
		"--repo", core,
		"--stream", "runs",
		"--event-id", "evt-1",
		"--payload", `{"status":"ok"}`,
		"--schema-ref", "schema/policy",
	), "SCHEMA_REVISION_UNRESOLVED")
	body(t, kc(h, "put",
		"--command-id", "schema-policy",
		"--repo", core,
		"--object", "schema/policy",
		"--value", `{"entity":"Policy","aspect":"structure","pattern":"record"}`,
	))
	proposal := asMap(t, body(t, kc(h, "propose",
		"--proposal-id", "PR-schema-ok",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema-ok",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	)))
	if proposal["candidateCommit"] == "" {
		t.Fatal(proposal)
	}
	appended := asMap(t, body(t, kc(h, "append",
		"--command-id", "run-ok",
		"--repo", core,
		"--stream", "runs",
		"--event-id", "evt-1",
		"--payload", `{"status":"ok"}`,
		"--schema-ref", "schema/policy",
	)))
	if asCursor(t, asMap(t, appended["result"])["cursor"]) == "" {
		t.Fatal(appended)
	}
}

func TestWritePath(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	expectCode(t, kc(h, "put",
		"--command-id", "missing-schema",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy@c1",
	), "SCHEMA_REVISION_UNRESOLVED")
	body(t, kc(h, "put",
		"--command-id", "schema-policy",
		"--repo", "kr://acme/public/core",
		"--object", "schema/policy",
		"--value", `{"entity":"Policy","aspect":"structure","pattern":"record"}`,
	))
	created := asMap(t, body(t, kc(h, "put",
		"--command-id", "create-a",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--if-absent",
		"--schema-ref", "schema/policy",
		"--origin-kind", "SOURCE",
		"--actor-ref", "alice",
	)))
	if created["disposition"] != "APPLIED" {
		t.Fatal(created)
	}
	dup := kc(h, "put",
		"--command-id", "create-a-again",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":2}`,
		"--if-absent",
	)
	expectCode(t, dup, "PRECONDITION_FAILED")
	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := "---\nobject_id: runbooks/oncall\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(h, "cs.json")
	preview := asMap(t, body(t, kc(h, "ingest", "--repo", "kr://acme/public/core", "--dir", draft, "--out", out)))
	files := preview["files"].([]any)
	if asMap(t, files[0])["objectId"] != "runbooks/oncall" {
		t.Fatal(preview["files"])
	}
	committed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--changeset", out)))
	if committed["disposition"] != "APPLIED" {
		t.Fatal(committed)
	}
	receipt := asMap(t, body(t, kc(h, "receipt", "--command-id", "ingest-1")))
	if receipt["commandId"] != "ingest-1" || receipt["digest"] == "" {
		t.Fatal(receipt)
	}
	missing := kc(h, "put",
		"--command-id", "derived-bad",
		"--repo", "kr://acme/public/core",
		"--object", "derived/x",
		"--value", `{"v":1}`,
		"--origin-kind", "DERIVATION",
	)
	expectCode(t, missing, "PRECONDITION_FAILED")
	body(t, kc(h, "put",
		"--command-id", "derived-ok",
		"--repo", "kr://acme/public/core",
		"--object", "derived/x",
		"--value", `{"v":1}`,
		"--origin-kind", "DERIVATION",
		"--input-vrv", "vr-1",
		"--algorithm-hash", "abc",
	))
}

func TestAuditTrail(t *testing.T) {
	h := testkit.TempDir(t)
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/core"), "no kc home")
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	kc(h, "repo-add", "--repo", "kr://acme/public/core")
	body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "policy/P-1",
		"--value", `{"secret":true}`,
	))
	expectCode(t, kc(h, "put", "--as", "other", "--command-id", "x", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"), "FORBIDDEN")

	trail := asMap(t, body(t, kc(h, "audit", "--layer", "kc")))
	entries := trail["entries"].([]any)
	cmds := make([]string, 0, len(entries))
	var putOk, putDenied map[string]any
	for _, item := range entries {
		row := asMap(t, item)
		cmds = append(cmds, row["cmd"].(string))
		if row["cmd"] == "put" && row["status"] == "ok" {
			putOk = row
		}
		if row["cmd"] == "put" && row["status"] == "error" {
			putDenied = row
		}
	}
	joined := strings.Join(cmds, " ")
	for _, want := range []string{"repo-add", "init", "repo-add", "put", "put"} {
		if !strings.Contains(joined, want) {
			t.Fatal(cmds)
		}
	}
	if asMap(t, entries[1])["cmd"] != "init" || asMap(t, entries[1])["layer"] != "kc" {
		t.Fatal(entries)
	}
	if asMap(t, asMap(t, entries[1])["refs"])["catalog"] != "kr://acme/catalog" {
		t.Fatal(entries[1])
	}
	if putOk == nil || asMap(t, putOk["args"])["value"] != "<redacted>" {
		t.Fatal(putOk)
	}
	if asMap(t, putOk["refs"])["newCommit"] == "" || asMap(t, putOk["refs"])["disposition"] != "APPLIED" {
		t.Fatal(putOk)
	}
	if putDenied == nil || asMap(t, putDenied["error"])["code"] != "FORBIDDEN" {
		t.Fatal(putDenied)
	}

	inits := asMap(t, body(t, kc(h, "audit", "--layer", "kc", "--cmd", "init")))
	if len(inits["entries"].([]any)) != 1 {
		t.Fatal(inits)
	}

	sys := asMap(t, body(t, kc(h, "audit", "--layer", "system")))
	sawCommit, sawCatalogInit := false, false
	for _, item := range sys["entries"].([]any) {
		row := asMap(t, item)
		if row["layer"] != "system" {
			t.Fatal(row)
		}
		switch row["cmd"] {
		case "COMMIT":
			if row["face"] == "writer" && asMap(t, row["refs"])["newCommit"] != "" {
				sawCommit = true
			}
		case "init":
			if row["face"] == "catalog" {
				sawCatalogInit = true
			}
		}
	}
	if !sawCommit || !sawCatalogInit {
		t.Fatal(sys)
	}

	catalogLog := asMap(t, body(t, kc(h, "audit")))
	sawInit := false
	for _, item := range catalogLog["entries"].([]any) {
		if strings.HasPrefix(asMap(t, item)["message"].(string), "init kr://acme/catalog") {
			sawInit = true
		}
	}
	if !sawInit {
		t.Fatal(catalogLog)
	}

	again := asMap(t, body(t, kc(h, "audit")))
	for _, item := range again["entries"].([]any) {
		if asMap(t, item)["cmd"] == "audit" {
			t.Fatal("audit must not log itself", again)
		}
	}
}
