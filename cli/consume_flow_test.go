package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestConsumeViewFollowsPublishedBranch(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "task-io", "--repo", core,
		"--object", "ETLTask:daily-orders", "--aspect", "io", "--value", `{"inputs":["orders"]}`))
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	c1 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "v1",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"body":"needs a runbook"}`,
	)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later live 冻结窗口"}`)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "2", "--source", core+"=refs/heads/main"))

	serving := body(t, kc(h, "read", "--dataset", "agent", "--object", "policy/A")).([]any)
	if len(serving) != 1 {
		t.Fatal(serving)
	}
	got := asMap(t, asMap(t, serving[0])["value"])
	if got["body"] != "later live 冻结窗口" {
		t.Fatal("consumer read uses the republished dataset version", got)
	}
	if asMap(t, serving[0])["commit"] != c2 {
		t.Fatal("result still names the resolved commit; caller did not pass it", serving[0], c1, c2)
	}

	space := asMap(t, body(t, kc(h, "catalog-show")))
	if space["catalogId"] != "kr://acme/catalog" {
		t.Fatal(space)
	}
	if _, ok := space["releases"]; ok {
		t.Fatal("read --catalog must not list releases", space)
	}
	if _, ok := space["generations"]; ok {
		t.Fatal("read --catalog must not list generations", space)
	}
	expectCode(t, kc(h, "show", "--as", "bot"), "FORBIDDEN")

	resolvedObject := body(t, kc(h, "resolve", "--dataset", "agent", "--object", "policy/A")).([]any)
	if len(resolvedObject) != 1 || asMap(t, resolvedObject[0])["status"] != "RESOLVED" {
		t.Fatalf("knowledge resolve: %#v", resolvedObject)
	}
	if asMap(t, resolvedObject[0])["commit"] != c2 {
		t.Fatalf("knowledge resolve must freeze this command's pin: %#v", resolvedObject)
	}
	aspectResolved := body(t, kc(h, "resolve", "--dataset", "agent",
		"--object", "ETLTask:daily-orders", "--aspect", "io")).([]any)
	if len(aspectResolved) != 1 || asMap(t, aspectResolved[0])["status"] != "RESOLVED" {
		t.Fatalf("knowledge resolve --aspect: %#v", aspectResolved)
	}
	missingAspect := body(t, kc(h, "resolve", "--dataset", "agent",
		"--object", "ETLTask:daily-orders", "--aspect", "missing")).([]any)
	if len(missingAspect) != 0 {
		t.Fatalf("workspace resolve of a missing Address is an empty union: %#v", missingAspect)
	}
	absent := body(t, kc(h, "resolve", "--dataset", "agent", "--object", "missing/nope")).([]any)
	if len(absent) != 0 {
		t.Fatalf("workspace resolve of a missing object is an empty union, not UNRESOLVED error: %#v", absent)
	}
	schemaReports := body(t, kc(h, "describe-schema", "--dataset", "agent", "--object", "schema/policy.body")).([]any)
	if len(schemaReports) != 1 || len(asMap(t, schemaReports[0])["schemas"].([]any)) != 1 {
		t.Fatalf("describe-schema must inspect the same pinned Workspace: %#v", schemaReports)
	}
	logsPage := asMap(t, body(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A")))
	logs := logsPage["logs"].([]any)
	if logsPage["exhausted"] != nil || len(logs) != 1 {
		t.Fatalf("log --workspace: %#v", logsPage)
	}
	log0 := asMap(t, logs[0])
	if log0["commit"] != c2 {
		t.Fatalf("object log must name the resolved commit: %#v", log0)
	}
	firstLog := asMap(t, body(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--limit", "1")))
	if firstLog["exhausted"] != nil || firstLog["continuation"] == "" {
		t.Fatalf("workspace object log must page: %#v", firstLog)
	}
	nextLog := asMap(t, body(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--limit", "1",
		"--continuation", firstLog["continuation"].(string))))
	if len(asMap(t, nextLog["logs"].([]any)[0])["revisions"].([]any)) == 0 {
		t.Fatalf("workspace log continuation: %#v", nextLog)
	}
	zeroLog := asMap(t, body(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--limit", "0")))
	if zeroLog["exhausted"] != nil || len(asMap(t, zeroLog["logs"].([]any)[0])["revisions"].([]any)) < 2 {
		t.Fatalf("workspace --limit 0 must mean the default history page: %#v", zeroLog)
	}
	expectCode(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--limit", "201"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--dataset", "agent", "--object", "policy/A", "--member", "user:bob"), "USAGE_INVALID")
	expectCode(t, kc(h, "provenance", "--dataset", "agent", "--object", "policy/A", "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "resolve", "--dataset", "agent", "--object", "policy/A", "--member", "user:bob"), "USAGE_INVALID")

	read0 := asMap(t, serving[0])
	if read0["objectId"] != "policy/A" {
		t.Fatalf("read --dataset must share product identity: %#v", read0)
	}
	if searchAvailable() {
		syncIndexes(t, h, core)
		search := asMap(t, body(t, kc(h, "search", "--dataset", "agent", "--query", "later")))
		hits := search["hits"].([]any)
		if len(hits) != 1 {
			t.Fatalf("search --dataset: %#v", hits)
		}
		if search["completeness"] != "complete" {
			t.Fatalf("exact dataset search must be complete: %#v", search)
		}
		cjkSearch := asMap(t, body(t, kc(h, "search", "--dataset", "agent", "--query", "冻结窗口")))
		if cjkSearch["completeness"] != "complete" || len(cjkSearch["hits"].([]any)) != 1 {
			t.Fatalf("declared text search must support contiguous CJK text: %#v", cjkSearch)
		}
		hit0 := asMap(t, hits[0])
		if hit0["objectId"] != "policy/A" {
			t.Fatalf("search envelope: %#v", hit0)
		}
	} else {
		expectCode(t, kc(h, "search", "--dataset", "agent", "--query", "later"), "CAPABILITY_UNSATISFIED")
	}

	catState := asMap(t, body(t, kc(h, "catalog-show")))
	if catState["catalogId"] == "" {
		t.Fatalf("catalog show: %#v", catState)
	}
	access := asMap(t, body(t, kc(h, "describe-access", "--dataset", "agent")))
	if len(access["specs"].([]any)) != 1 {
		t.Fatalf("access describe: %#v", access)
	}
	if searchAvailable() {
		pinnedIndex := asMap(t, body(t, kc(h, "describe-index", "--repo", core, "--commit", c2)))
		if pinnedIndex["basisCommit"] != c2 || pinnedIndex["lagBehindHead"] != false {
			t.Fatalf("projection must describe this Dataset commit, not a stale live index: %#v pin %s", pinnedIndex, c2)
		}
		desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
		if desc["basisCommit"] != c2 {
			t.Fatalf("live index follows HEAD: %#v", desc)
		}
		syncedIndex := asMap(t, body(t, kc(h, "index-sync", "--repo", core, "--commit", c2)))
		if syncedIndex["basisCommit"] != c2 || syncedIndex["mode"] != "ready" {
			t.Fatalf("index-sync must report the requested ready basis: %#v", syncedIndex)
		}
	}

	hist := asMap(t, body(t, kc(h, "audit", "--dataset", "agent")))
	if hist["source"] != "catalog" {
		t.Fatal("workspace-filtered registry history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog", "kr://acme/catalog"), "rejects --catalog")
	expectMsg(t, kc(h, "log", "--dataset", "agent"), "missing --object")
	expectMsg(t, kc(h, "log", "--dataset", "agent", "--repo", core, "--object", "policy/A"), "do not mix")

	live := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["body"] != "later live 冻结窗口" {
		t.Fatal("maintainer read --repo still follows the named ref", live)
	}

	expectCode(t, kc(h, "read", "--as", "bot", "--dataset", "agent", "--object", "policy/A"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--dataset", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := body(t, kc(h, "read", "--as", "bot", "--dataset", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, asBot[0])["value"])["body"] != "later live 冻结窗口" {
		t.Fatal(asBot)
	}
	expectCode(t, kc(h, "show", "--as", "bot"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", "kr://acme/catalog"))
	asBotSpace := asMap(t, body(t, kc(h, "show", "--as", "bot")))
	if asBotSpace["catalogId"] != "kr://acme/catalog" {
		t.Fatal(asBotSpace)
	}

	expectMsg(t, kc(h, "read", "--dataset", "agent", "--repo", core, "--object", "policy/A"), "do not mix")
	expectMsg(t, kc(h, "read", "--dataset", "agent", "--commit", c1, "--object", "policy/A"), "do not mix")
	expectCode(t, kc(h, "list", "--dataset", "agent", "--ref", "refs/heads/main"), "USAGE_INVALID")
	expectCode(t, kc(h, "read", "--dataset", "missing", "--object", "policy/A"), "KNOWLEDGE_SET_INVALID")
	expectMsg(t, kc(h, "promote", "--dataset", "agent"), "unknown command promote")
	expectMsg(t, kc(h, "read", "--release", "stable", "--object", "policy/A"), "unknown flag --release")
}

func TestKnowledgeSetAuthorizationCoverageIsHonest(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	public := "kr://acme/public/runbooks"
	private := "kr://acme/private/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	for _, repo := range []string{public, private} {
		seedRepo(t, h, repo)
		body(t, kc(h, "put", "--command-id", "schema-"+repo, "--repo", repo,
			"--object", "schema/runbook.body",
			"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	}
	body(t, kc(h, "put", "--command-id", "public-body", "--repo", public,
		"--object", "runbook/public", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "put", "--command-id", "private-body", "--repo", private,
		"--object", "runbook/private", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment private procedure"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", public+"=refs/heads/main", "--source", private+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "file.read,dataset.resolve,knowledge.search",
		"--catalog", catalogID, "--dataset", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", public))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.history.read", "--repo", public))

	expectCode(t, kc(h, "read", "--as", "bot", "--repo", private, "--object", "runbook/private"), "FORBIDDEN")
	body(t, kc(h, "read", "--as", "bot", "--repo", public, "--object", "runbook/public"))

	for _, objectID := range []string{"runbook/private", "runbook/public"} {
		reads := body(t, kc(h, "read", "--as", "bot", "--dataset", "agent", "--object", objectID)).([]any)
		if len(reads) == 0 {
			t.Fatalf("dataset file.read must deliver listed files: %s", objectID)
		}
	}

	if searchAvailable() {
		syncIndexes(t, h, public)
		syncIndexes(t, h, private)
		search := asMap(t, body(t, kc(h, "search", "--as", "bot", "--dataset", "agent", "--query", "payment")))
		if search["completeness"] != "complete" {
			t.Fatalf("dataset search must be complete: %#v", search)
		}
		snapshots := asMap(t, asMap(t, search["searchView"])["snapshots"])
		if snapshots[public] == nil || snapshots[private] == nil {
			t.Fatalf("SearchView must keep dataset members: %#v", search)
		}
		hits := search["hits"].([]any)
		if len(hits) != 2 {
			t.Fatalf("SEARCH must return both members: %#v", search)
		}
		seen := map[string]bool{}
		for _, raw := range hits {
			hit := asMap(t, raw)
			repo := hit["repository"].(string)
			seen[repo] = true
			if hit["objectId"] == "" {
				t.Fatalf("hit must keep coordinates: %#v", hit)
			}
			if hit["value"] != nil {
				t.Fatalf("CLI search lists identity; Canonical stays on READ: %#v", hit)
			}
		}
		if !seen[public] || !seen[private] {
			t.Fatalf("SEARCH must keep dataset members: %#v", search)
		}
	} else {
		expectCode(t, kc(h, "search", "--as", "bot", "--dataset", "agent", "--query", "payment"), "CAPABILITY_UNSATISFIED")
	}
}

func TestCatalogReadDiscoversWithoutKnowledgeRead(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", repo,
		"--object", "runbook/public", "--value", `{"body":"secret procedure"}`))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", catalogID))

	state := asMap(t, body(t, kc(h, "show", "--as", "bot")))
	listed := businessRepositories(state)
	if len(listed) != 1 || listed[0] != repo {
		t.Fatalf("catalog.read must discover registered repositories: %#v", state)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--repo", repo, "--object", "runbook/public"), "FORBIDDEN")
}

func TestRepoSearchDeliveryStripsUnauthorizedBody(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", "--command-id", "schema", "--repo", repo,
		"--object", "schema/runbook.body",
		"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "body", "--repo", repo,
		"--object", "runbook/public", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.search", "--repo", repo))
	if !searchAvailable() {
		expectCode(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment"), "CAPABILITY_UNSATISFIED")
		return
	}
	syncIndexes(t, h, repo)

	search := asMap(t, body(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment")))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("SEARCH must still locate the object: %#v", search)
	}
	hit := asMap(t, hits[0])
	if hit["objectId"] != "runbook/public" || hit["value"] != nil {
		t.Fatalf("CLI search lists identity without Canonical body: %#v", search)
	}

	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.read", "--repo", repo))
	granted := asMap(t, body(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment")))
	grantedHit := asMap(t, granted["hits"].([]any)[0])
	if grantedHit["objectId"] != "runbook/public" || grantedHit["value"] != nil {
		t.Fatalf("CLI search still lists identity after knowledge.read: %#v", granted)
	}
}

func TestHTTPWorkspaceSearchStripsUnauthorizedBody(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", "--command-id", "schema", "--repo", repo,
		"--object", "schema/runbook.body",
		"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "body", "--repo", repo,
		"--object", "runbook/public", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repo+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "taihu:alice", "--action", "file.read",
		"--catalog", catalogID, "--dataset", "agent"))
	body(t, kc(h, "allow", "--principal", "taihu:alice", "--action", "knowledge.search", "--repo", repo))
	if !searchAvailable() {
		expectCode(t, kc(h, "search", "--as", "taihu:alice", "--dataset", "agent", "--query", "payment"), "CAPABILITY_UNSATISFIED")
		return
	}
	syncIndexes(t, h, repo)

	server := httptest.NewServer(cli.HTTPHandler(h))
	t.Cleanup(server.Close)
	if closer, ok := server.Config.Handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	raw, err := json.Marshal(map[string]any{"dataset": "agent", "query": "payment"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/search", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "taihu:alice")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyBytes, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SEARCH status=%d body=%s", response.StatusCode, bodyBytes)
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	hits, _ := payload["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("SEARCH must still locate the object: %#v", payload)
	}
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	if knowledge["value"] != nil {
		t.Fatalf("unauthorized Canonical body escaped workspace HTTP SEARCH: %#v", payload)
	}
	if asMap(t, knowledge["knowledgeRef"])["object"] == "" {
		t.Fatalf("masked hit must keep coordinates: %#v", hits[0])
	}
}

func TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	group := "kr://acme/groups/payments"
	catalogID := "kr://acme/catalog"
	body(t, kc(h, "init", "--catalog", catalogID))
	seedRepo(t, h, core)
	seedRepo(t, h, group)
	body(t, kc(h, "put",
		"--command-id", "pub",
		"--repo", core,
		"--object", "policy/P-103",
		"--value", `{"body":"public"}`,
	))
	body(t, kc(h, "put", "--command-id", "grp", "--repo", group, "--object", "policy/P-103", "--value", `{"body":"group"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "payments-agent", "--revision", "1",
		"--source", core+"=refs/heads/main",
		"--source", group+"=refs/heads/main"))
	body(t, kc(h, "grant", "add", "--principal", "agent:files",
		"--action", "dataset.resolve", "--catalog", catalogID, "--dataset", "payments-agent"))

	server := httptest.NewServer(cli.HTTPHandler(h))
	t.Cleanup(server.Close)
	if closer, ok := server.Config.Handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	raw, _ := json.Marshal(map[string]any{"dataset": "payments-agent"})
	request, err := http.NewRequest(http.MethodPost, server.URL+"/dataset-files/v1/mounts:list", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:files")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyBytes, _ := io.ReadAll(response.Body)
	if response.StatusCode == http.StatusOK {
		t.Fatalf("knowledge-only workspace must not project a file tree: %s", bodyBytes)
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatal(err, string(bodyBytes))
	}
	if asMap(t, payload["error"])["code"] != "CAPABILITY_UNSATISFIED" {
		t.Fatalf("file gateway must fail closed instead of scanning knowledge objects: %#v", payload)
	}
}

func TestDatasetSearchExcludesPathsOutsidePublishedList(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "listed", "--repo", core, "--object", "listed",
		"--schema-ref", "schema/policy.body", "--path-hint", "policies/listed.yaml",
		"--value", `{"body":"listed payment procedure"}`))
	body(t, kc(h, "put", "--command-id", "schema-note", "--repo", core, "--object", "schema/note.body",
		"--value", `{"entity":"Note","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "secret", "--repo", core, "--object", "secret",
		"--schema-ref", "schema/note.body", "--path-hint", "notes/secret.yaml",
		"--value", `{"body":"secret payment procedure"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "docs", "--revision", "1",
		"--source", core+"=refs/heads/main@policies@policies"))
	listed := body(t, kc(h, "read", "--dataset", "docs", "--object", "listed")).([]any)
	if len(listed) != 1 {
		t.Fatalf("prefix dataset must read listed files: %#v", listed)
	}
	secret := body(t, kc(h, "read", "--dataset", "docs", "--object", "secret")).([]any)
	if len(secret) != 0 {
		t.Fatalf("prefix dataset must not read files outside the published list: %#v", secret)
	}
	if searchAvailable() {
		syncIndexes(t, h, core)
		search := asMap(t, body(t, kc(h, "search", "--dataset", "docs", "--query", "payment")))
		hits := search["hits"].([]any)
		if len(hits) != 1 || asMap(t, hits[0])["objectId"] != "listed" {
			t.Fatalf("SEARCH must not return paths outside the serving file list: %#v", search)
		}
	} else {
		expectCode(t, kc(h, "search", "--dataset", "docs", "--query", "payment"), "CAPABILITY_UNSATISFIED")
	}
}

func TestHistoricalDatasetReadStaysOnPublishedCommit(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	frozenCommit := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v1", "--repo", core, "--object", "policy/A",
		"--schema-ref", "schema/policy.body", "--value", `{"body":"frozen payment"}`)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	if searchAvailable() {
		syncIndexes(t, h, core)
	}
	laterCommit := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A",
		"--schema-ref", "schema/policy.body", "--value", `{"body":"later live payment"}`)))["result"])["newCommit"].(string)
	if searchAvailable() {
		syncIndexes(t, h, core)
	}
	frozen := body(t, kc(h, "read", "--dataset", "agent", "--object", "policy/A")).([]any)
	if len(frozen) != 1 || asMap(t, asMap(t, frozen[0])["value"])["body"] != "frozen payment" {
		t.Fatalf("published dataset READ must stay on Snapshot: %#v", frozen)
	}
	if asMap(t, frozen[0])["commit"] != frozenCommit {
		t.Fatalf("published dataset READ must name the frozen commit: %#v", frozen)
	}
	search := kc(h, "search", "--dataset", "agent", "--query", "payment")
	if search.Status != 0 {
		expectCode(t, search, "CAPABILITY_UNSATISFIED")
		return
	}
	hits := asMap(t, body(t, search))["hits"].([]any)
	for _, raw := range hits {
		if asMap(t, raw)["commit"] == laterCommit {
			t.Fatalf("published dataset SEARCH must not use a later serving commit: %#v", hits)
		}
	}
}

func searchAvailable() bool {
	return os.Getenv("KC_TEST_OPENSEARCH_URL") != ""
}
