package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestConsumeViewFollowsPublishedBranch(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
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
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later live 冻结窗口"}`)))["result"])["newCommit"].(string)

	serving := body(t, kc(h, "read", "--workspace", "agent", "--object", "policy/A")).([]any)
	if len(serving) != 1 {
		t.Fatal(serving)
	}
	got := asMap(t, asMap(t, serving[0])["value"])
	if got["body"] != "later live 冻结窗口" {
		t.Fatal("consumer read follows the published branch", got)
	}
	if asMap(t, serving[0])["commit"] != c2 {
		t.Fatal("result still names the resolved commit; caller did not pass it", serving[0], c1, c2)
	}

	space := asMap(t, body(t, kc(h, "read", "--catalog")))
	if space["catalogId"] != "kr://acme/catalog" {
		t.Fatal(space)
	}
	if _, ok := space["releases"]; ok {
		t.Fatal("read --catalog must not list releases", space)
	}
	if _, ok := space["generations"]; ok {
		t.Fatal("read --catalog must not list generations", space)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--catalog"), "FORBIDDEN")

	pin := asMap(t, body(t, kc(h, "resolve", "--workspace", "agent")))
	if pin["workspaceId"] != "agent" {
		t.Fatalf("resolve --workspace pin: %#v", pin)
	}
	if _, legacy := pin["appendCuts"]; legacy {
		t.Fatalf("Snapshot pin must not carry dynamic observation cuts: %#v", pin)
	}
	if asMap(t, pin["repositories"])[core] != c2 {
		t.Fatalf("pin must name this command's commits: %#v", pin)
	}
	resolvedObject := body(t, kc(h, "knowledge", "resolve", "--workspace", "agent", "--object", "policy/A")).([]any)
	if len(resolvedObject) != 1 || asMap(t, resolvedObject[0])["status"] != "RESOLVED" {
		t.Fatalf("knowledge resolve: %#v", resolvedObject)
	}
	if asMap(t, resolvedObject[0])["commit"] != c2 {
		t.Fatalf("knowledge resolve must freeze this command's pin: %#v", resolvedObject)
	}
	aspectResolved := body(t, kc(h, "knowledge", "resolve", "--workspace", "agent",
		"--object", "ETLTask:daily-orders", "--aspect", "io")).([]any)
	if len(aspectResolved) != 1 || asMap(t, aspectResolved[0])["status"] != "RESOLVED" {
		t.Fatalf("knowledge resolve --aspect: %#v", aspectResolved)
	}
	missingAspect := body(t, kc(h, "knowledge", "resolve", "--workspace", "agent",
		"--object", "ETLTask:daily-orders", "--aspect", "missing")).([]any)
	if len(missingAspect) != 0 {
		t.Fatalf("workspace resolve of a missing Address is an empty union: %#v", missingAspect)
	}
	absent := body(t, kc(h, "knowledge", "resolve", "--workspace", "agent", "--object", "missing/nope")).([]any)
	if len(absent) != 0 {
		t.Fatalf("workspace resolve of a missing object is an empty union, not UNRESOLVED error: %#v", absent)
	}
	expectCode(t, kc(h, "catalog", "workspace", "resolve", "--workspace", "agent", "--object", "policy/A"), "USAGE_INVALID")
	expectCode(t, kc(h, "catalog", "workspace", "resolve", "--workspace", "agent", "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "catalog", "workspace", "resolve", "--workspace", "agent", "--member", "user:bob"), "USAGE_INVALID")
	schemaReports := body(t, kc(h, "describe-schema", "--workspace", "agent", "--object", "schema/policy.body")).([]any)
	if len(schemaReports) != 1 || len(asMap(t, schemaReports[0])["schemas"].([]any)) != 1 {
		t.Fatalf("describe-schema must inspect the same pinned Workspace: %#v", schemaReports)
	}
	logsPage := asMap(t, body(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A")))
	logs := logsPage["logs"].([]any)
	if logsPage["exhausted"] != true || len(logs) != 1 {
		t.Fatalf("log --workspace: %#v", logsPage)
	}
	log0 := asMap(t, logs[0])
	if log0["commit"] != c2 {
		t.Fatalf("object log must name the resolved commit: %#v", log0)
	}
	firstLog := asMap(t, body(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--limit", "1")))
	if firstLog["exhausted"] == true || firstLog["continuation"] == "" {
		t.Fatalf("workspace object log must page: %#v", firstLog)
	}
	nextLog := asMap(t, body(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--limit", "1",
		"--continuation", firstLog["continuation"].(string))))
	if len(asMap(t, nextLog["logs"].([]any)[0])["revisions"].([]any)) == 0 {
		t.Fatalf("workspace log continuation: %#v", nextLog)
	}
	zeroLog := asMap(t, body(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--limit", "0")))
	if zeroLog["exhausted"] != true || len(asMap(t, zeroLog["logs"].([]any)[0])["revisions"].([]any)) < 2 {
		t.Fatalf("workspace --limit 0 must mean the default history page: %#v", zeroLog)
	}
	expectCode(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--limit", "201"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A", "--member", "user:bob"), "USAGE_INVALID")
	expectCode(t, kc(h, "knowledge", "provenance", "--workspace", "agent", "--object", "policy/A", "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, kc(h, "knowledge", "resolve", "--workspace", "agent", "--object", "policy/A", "--member", "user:bob"), "USAGE_INVALID")

	syncIndexes(t, h, core)
	search := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--query", "later")))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("search --workspace: %#v", hits)
	}
	if search["completeness"] != "complete" {
		t.Fatalf("exact workspace search must be complete: %#v", search)
	}
	cjkSearch := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--query", "冻结窗口")))
	if cjkSearch["completeness"] != "complete" || len(cjkSearch["hits"].([]any)) != 1 {
		t.Fatalf("declared text search must support contiguous CJK text: %#v", cjkSearch)
	}
	hit0 := asMap(t, asMap(t, hits[0])["knowledge"])
	if asMap(t, hit0["knowledgeRef"])["object"] != "policy/A" {
		t.Fatalf("search envelope: %#v", hit0)
	}
	read0 := asMap(t, serving[0])
	if asMap(t, read0["knowledgeRef"])["object"] != "policy/A" || asMap(t, read0["address"])["objectId"] != "policy/A" {
		t.Fatalf("read --workspace must share KnowledgeValue fields: %#v", read0)
	}

	catState := asMap(t, body(t, kc(h, "catalog", "show")))
	if catState["catalogId"] == "" {
		t.Fatalf("catalog show: %#v", catState)
	}
	pinView := asMap(t, body(t, kc(h, "resolve", "--workspace", "agent")))
	if pinView["workspaceId"] != "agent" {
		t.Fatalf("workspace pin: %#v", pinView)
	}
	access := asMap(t, body(t, kc(h, "describe-access", "--workspace", "agent")))
	if len(access["specs"].([]any)) != 1 {
		t.Fatalf("access describe: %#v", access)
	}
	pinCommit := asMap(t, pinView["repositories"])[core].(string)
	pinnedIndex := asMap(t, body(t, kc(h, "describe-index", "--repo", core, "--commit", pinCommit)))
	if pinnedIndex["basisCommit"] != c2 || pinnedIndex["lagBehindHead"] != false {
		t.Fatalf("pin projection must describe this Workspace pin, not a stale live index: %#v pin %s", pinnedIndex, c2)
	}
	desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
	if desc["basisCommit"] != c2 {
		t.Fatalf("live index follows HEAD: %#v", desc)
	}
	syncedIndex := asMap(t, body(t, kc(h, "index-sync", "--repo", core, "--commit", c2)))
	if syncedIndex["basisCommit"] != c2 || syncedIndex["mode"] != "ready" {
		t.Fatalf("index-sync must report the requested ready basis: %#v", syncedIndex)
	}

	hist := asMap(t, body(t, kc(h, "audit", "--workspace", "agent")))
	if hist["source"] != "catalog" {
		t.Fatal("workspace-filtered registry history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog"), "kc audit")
	expectMsg(t, kc(h, "log", "--workspace", "agent"), "missing --object")
	expectMsg(t, kc(h, "log", "--workspace", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")

	live := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["body"] != "later live 冻结窗口" {
		t.Fatal("maintainer read --repo still follows the named ref", live)
	}

	expectCode(t, kc(h, "read", "--as", "bot", "--workspace", "agent", "--object", "policy/A"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := body(t, kc(h, "read", "--as", "bot", "--workspace", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, asBot[0])["value"])["body"] != "later live 冻结窗口" {
		t.Fatal(asBot)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--catalog"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", "kr://acme/catalog"))
	asBotSpace := asMap(t, body(t, kc(h, "read", "--as", "bot", "--catalog")))
	if asBotSpace["catalogId"] != "kr://acme/catalog" {
		t.Fatal(asBotSpace)
	}

	expectMsg(t, kc(h, "read", "--workspace", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")
	expectMsg(t, kc(h, "read", "--workspace", "agent", "--commit", c1, "--object", "policy/A"), "cannot be combined")
	expectCode(t, kc(h, "list", "--workspace", "agent", "--ref", "refs/heads/main"), "USAGE_INVALID")
	expectCode(t, kc(h, "read", "--workspace", "missing", "--object", "policy/A"), "WORKSPACE_INVALID")
	expectMsg(t, kc(h, "promote", "--workspace", "agent"), "unknown command promote")
	expectMsg(t, kc(h, "read", "--release", "stable", "--object", "policy/A"), "unknown flag --release")
}

func TestWorkspaceAuthorizationCoverageIsHonest(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	public := "kr://acme/public/runbooks"
	private := "kr://acme/private/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	for _, repo := range []string{public, private} {
		body(t, kc(h, "repo-add", "--repo", repo))
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
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", public+"=refs/heads/main", "--source", private+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace",
		"--catalog", catalogID, "--workspace", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", public))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.history.read", "--repo", public))

	expectCode(t, kc(h, "search", "--as", "bot", "--workspace", "agent", "--query", "payment"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.search",
		"--catalog", catalogID, "--workspace", "agent"))

	// Bare-array reads cannot honestly represent partial coverage, so they fail
	// closed instead of making a hidden member look like an absent object.
	// That includes objects that live only in the authorized member: skipping
	// the hidden repository and returning the visible subset would still be a
	// silent authorization clip.
	for _, objectID := range []string{"runbook/private", "runbook/public"} {
		expectCode(t, kc(h, "read", "--as", "bot", "--workspace", "agent",
			"--object", objectID), "FORBIDDEN")
		expectCode(t, kc(h, "knowledge", "resolve", "--as", "bot", "--workspace", "agent",
			"--object", objectID), "FORBIDDEN")
		expectCode(t, kc(h, "knowledge", "log", "--as", "bot", "--workspace", "agent",
			"--object", objectID), "FORBIDDEN")
		expectCode(t, kc(h, "knowledge", "provenance", "--as", "bot", "--workspace", "agent",
			"--object", objectID), "FORBIDDEN")
	}
	expectCode(t, kc(h, "relations", "--as", "bot", "--workspace", "agent",
		"--object", "kc://acme/private/runbooks/runbook/private"), "FORBIDDEN")
	expectCode(t, kc(h, "relations", "--as", "bot", "--workspace", "agent",
		"--object", "kc://acme/public/runbooks/runbook/public"), "FORBIDDEN")
	expectCode(t, kc(h, "resolve", "--as", "bot", "--workspace", "agent"), "FORBIDDEN")
	expectCode(t, kc(h, "describe-access", "--as", "bot", "--workspace", "agent"), "FORBIDDEN")

	// SEARCH discovers every pin member. Missing knowledge.read must not omit
	// the repo, mark partial, or hide it from SearchView; the delivery chain
	// strips Canonical body instead.
	syncIndexes(t, h, public)
	syncIndexes(t, h, private)
	search := asMap(t, body(t, kc(h, "search", "--as", "bot", "--workspace", "agent", "--query", "payment")))
	if search["completeness"] != "complete" {
		t.Fatalf("missing knowledge.read is not a completeness gap: %#v", search)
	}
	if _, ok := search["claims"]; ok {
		t.Fatalf("authorization must not add an omission claim: %#v", search)
	}
	snapshots := asMap(t, asMap(t, search["searchView"])["snapshots"])
	if snapshots[public] == nil || snapshots[private] == nil {
		t.Fatalf("SearchView must keep unauthorized members: %#v", search)
	}
	hits := search["hits"].([]any)
	if len(hits) != 2 {
		t.Fatalf("SEARCH must return both members: %#v", search)
	}
	bodies := map[string]any{}
	for _, raw := range hits {
		hit := asMap(t, raw)
		knowledge := asMap(t, hit["knowledge"])
		repo := knowledge["repository"].(string)
		bodies[repo] = knowledge["value"]
		if asMap(t, knowledge["knowledgeRef"])["object"] == "" {
			t.Fatalf("masked hit must keep coordinates: %#v", hit)
		}
	}
	if bodies[public] == nil {
		t.Fatalf("authorized hit lost its body: %#v", search)
	}
	if asMap(t, bodies[public])["body"] != "payment public procedure" {
		t.Fatalf("authorized body: %#v", search)
	}
	if bodies[private] != nil {
		t.Fatalf("unauthorized Canonical body escaped the delivery chain: %#v", search)
	}
}

func TestCatalogReadDiscoversWithoutKnowledgeRead(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	body(t, kc(h, "repo-add", "--repo", repo))
	body(t, kc(h, "put", "--command-id", "seed", "--repo", repo,
		"--object", "runbook/public", "--value", `{"body":"secret procedure"}`))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", catalogID))

	state := asMap(t, body(t, kc(h, "catalog", "show", "--as", "bot")))
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
	body(t, kc(h, "repo-add", "--repo", repo))
	body(t, kc(h, "put", "--command-id", "schema", "--repo", repo,
		"--object", "schema/runbook.body",
		"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "body", "--repo", repo,
		"--object", "runbook/public", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.search", "--repo", repo))
	syncIndexes(t, h, repo)

	search := asMap(t, body(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment")))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("SEARCH must still locate the object: %#v", search)
	}
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	if knowledge["value"] != nil {
		t.Fatalf("unauthorized Canonical body escaped --repo SEARCH: %#v", search)
	}
	if asMap(t, knowledge["knowledgeRef"])["object"] == "" {
		t.Fatalf("masked hit must keep coordinates: %#v", hits[0])
	}

	body(t, kc(h, "allow", "--principal", "bot", "--action", "knowledge.read", "--repo", repo))
	granted := asMap(t, body(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment")))
	value := asMap(t, asMap(t, asMap(t, granted["hits"].([]any)[0])["knowledge"])["value"])
	if value["body"] != "payment public procedure" {
		t.Fatalf("authorized --repo SEARCH lost its body: %#v", granted)
	}
}

func TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	group := "kr://acme/groups/payments"
	catalogID := "kr://acme/catalog"
	body(t, kc(h, "init", "--catalog", catalogID))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "repo-add", "--repo", group))
	body(t, kc(h, "put",
		"--command-id", "pub",
		"--repo", core,
		"--object", "policy/P-103",
		"--value", `{"body":"public"}`,
	))
	body(t, kc(h, "put", "--command-id", "grp", "--repo", group, "--object", "policy/P-103", "--value", `{"body":"group"}`))
	body(t, kc(h, "define-workspace", "--workspace", "payments-agent", "--revision", "1",
		"--source", core+"=refs/heads/main",
		"--source", group+"=refs/heads/main"))
	body(t, kc(h, "admin", "grant", "add", "--principal", "agent:files",
		"--action", "workspace.resolve", "--catalog", catalogID, "--workspace", "payments-agent"))

	server := httptest.NewServer(cli.HTTPHandler(h))
	t.Cleanup(server.Close)
	if closer, ok := server.Config.Handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	raw, _ := json.Marshal(map[string]any{"workspace": "payments-agent"})
	request, err := http.NewRequest(http.MethodPost, server.URL+"/workspace-files/v1/mounts:list", bytes.NewReader(raw))
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
