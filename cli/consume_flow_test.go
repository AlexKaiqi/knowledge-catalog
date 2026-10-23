package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/internal/testkit"
)

func TestConsumeViewFollowsPublishedBranch(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "etl-task/daily-orders", "/io"), "--repo", core, "--command-id", "task-io", "--value", `{"inputs":["orders"]}`))
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy.body", ""), "--repo", core, "--command-id", "schema-body", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"type":"string"}}}`))
	seedTextDeclaration(t, h, core, "index/policy-body", "schema/policy.body", "body")
	c1 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "v1", "--value", `{"body":"needs a runbook"}`)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "v2", "--value", `{"body":"later live 冻结窗口"}`)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "2", "--source", core+"=refs/heads/main"))

	serving := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--dataset", "agent")))
	if serving["reference"] != testObjectReference(t, core, "policy/A", "") {
		t.Fatal(serving)
	}
	got := asMap(t, asMap(t, serving)["value"])
	if got["body"] != "later live 冻结窗口" {
		t.Fatal("consumer read uses the republished dataset version", got)
	}
	if asMap(t, serving)["commit"] != c2 {
		t.Fatal("result still names the resolved commit; caller did not pass it", serving, c1, c2)
	}

	space := asMap(t, body(t, kc(h, "show")))
	if _, present := space["catalogId"]; present {
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
		t.Fatalf("object resolve: %#v", resolvedObject)
	}
	if asMap(t, resolvedObject[0])["commit"] != c2 {
		t.Fatalf("object resolve must freeze this command's pin: %#v", resolvedObject)
	}
	aspectResolved := body(t, kc(h, "resolve", "--dataset", "agent",
		"--object", "etl-task/daily-orders", "--aspect", "io")).([]any)
	if len(aspectResolved) != 1 || asMap(t, aspectResolved[0])["status"] != "RESOLVED" {
		t.Fatalf("object resolve --aspect: %#v", aspectResolved)
	}
	missingAspect := body(t, kc(h, "resolve", "--dataset", "agent",
		"--object", "etl-task/daily-orders", "--aspect", "missing")).([]any)
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

	read0 := asMap(t, serving)
	if read0["objectId"] != "policy/A" {
		t.Fatalf("read --dataset must share product identity: %#v", read0)
	}
	if searchAvailable() {
		syncIndexes(t, h, "agent")
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

	catState := asMap(t, body(t, kc(h, "show")))
	if _, present := catState["catalogId"]; present {
		t.Fatalf("catalog show: %#v", catState)
	}
	access := asMap(t, body(t, kc(h, "describe-access", "--dataset", "agent")))
	if len(access["specs"].([]any)) != 1 {
		t.Fatalf("access describe: %#v", access)
	}
	if searchAvailable() {
		pinnedReport := body(t, kc(h, "index", "describe", "--dataset", "agent", "--revision", "2"))
		pinnedIndex := datasetIndexBasis(t, pinnedReport, core)
		member := datasetIndexMember(t, pinnedReport, core)
		if member["commit"] != c2 || asMap(t, pinnedReport)["revision"] != float64(2) {
			t.Fatalf("index report lost frozen source coordinate: %#v", pinnedReport)
		}
		if asMap(t, pinnedReport)["ready"] != true || pinnedIndex["physicalDigest"] == "" || member["stage"] != "ready" {
			t.Fatalf("index must describe this Dataset commit, not a stale live index: %#v pin %s", pinnedIndex, c2)
		}
		desc := datasetIndexBasis(t, body(t, kc(h, "index", "describe", "--dataset", "agent")), core)
		if desc["physicalDigest"] != pinnedIndex["physicalDigest"] {
			t.Fatalf("live index follows HEAD: %#v", desc)
		}
		synced := asMap(t, body(t, kc(h, "index", "prepare", "--dataset", "agent", "--revision", "2")))
		syncedIndex := datasetIndexBasis(t, synced, core)
		if syncedIndex["physicalDigest"] != pinnedIndex["physicalDigest"] || syncedIndex["mode"] != "rebuild" || synced["ready"] != true {
			t.Fatalf("index prepare must report the requested ready basis: %#v", syncedIndex)
		}
	}

	hist := asMap(t, body(t, kc(h, "audit", "--dataset", "agent")))
	if hist["source"] != "registry" || hist["authorityId"] != "kr://kc/registry" || hist["catalogId"] != nil {
		t.Fatal("workspace-filtered registry history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog", "retired"), "rejects --catalog")
	expectMsg(t, kc(h, "log", "--dataset", "agent"), "missing --object")
	expectMsg(t, kc(h, "log", "--dataset", "agent", "--repo", core, "--object", "policy/A"), "do not mix")

	live := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["body"] != "later live 冻结窗口" {
		t.Fatal("maintainer read --repo still follows the named ref", live)
	}

	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--as", "bot", "--dataset", "agent"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--dataset", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--as", "bot", "--dataset", "agent")))
	if asMap(t, asMap(t, asBot)["value"])["body"] != "later live 冻结窗口" {
		t.Fatal(asBot)
	}
	expectCode(t, kc(h, "show", "--as", "bot"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--action", "inventory.read"))
	asBotSpace := asMap(t, body(t, kc(h, "show", "--as", "bot")))
	if _, present := asBotSpace["catalogId"]; present {
		t.Fatal(asBotSpace)
	}

	body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--dataset", "agent", "--commit", c2))
	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--dataset", "agent", "--commit", c1), "USAGE_INVALID")
	expectCode(t, kc(h, "list", "--dataset", "agent", "--ref", "refs/heads/main"), "USAGE_INVALID")
	expectCode(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--dataset", "missing"), "DATASET_INVALID")
	expectMsg(t, kc(h, "promote", "--dataset", "agent"), "unknown command promote")
	expectMsg(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--release", "stable"), "unknown flag --release")
}

func TestDatasetAuthorizationCoverageIsHonest(t *testing.T) {
	h := testkit.TempDir(t)
	public := "kr://acme/public/runbooks"
	private := "kr://acme/private/runbooks"
	body(t, kc(h, "init"))
	for _, repo := range []string{public, private} {
		seedRepo(t, h, repo)
		body(t, kc(h, "put", testObjectReference(t, repo, "schema/runbook."+map[string]string{public: "public", private: "private"}[repo], ""), "--repo", repo, "--command-id", "schema-"+repo, "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string"}}}`))
		seedTextDeclaration(t, h, repo, "index/runbook-"+map[string]string{public: "public", private: "private"}[repo], "schema/runbook."+map[string]string{public: "public", private: "private"}[repo], "body")
	}
	body(t, kc(h, "put", testObjectReference(t, public, "runbook/public", ""), "--repo", public, "--command-id", "public-body", "--schema-ref", "schema/runbook.public", "--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "put", testObjectReference(t, private, "runbook/private", ""), "--repo", private, "--command-id", "private-body", "--schema-ref", "schema/runbook.private", "--value", `{"body":"payment private procedure"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", public+"=refs/heads/main", "--source", private+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "file.read,dataset.resolve", "--dataset", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", public))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "object.history.read", "--repo", public))

	expectCode(t, kc(h, "read", testObjectReference(t, private, "runbook/private", ""), "--repo", private, "--as", "bot"), "FORBIDDEN")
	body(t, kc(h, "read", testObjectReference(t, public, "runbook/public", ""), "--repo", public, "--as", "bot"))

	for _, source := range []struct{ repository, object string }{{private, "runbook/private"}, {public, "runbook/public"}} {
		ref := testObjectReference(t, source.repository, source.object, "")
		read := asMap(t, body(t, kc(h, "read", ref, "--as", "bot", "--dataset", "agent")))
		if read["reference"] != ref || read["repository"] != source.repository || read["objectId"] != source.object {
			t.Fatalf("dataset file.read must deliver the selected source: %#v", read)
		}
	}

	if searchAvailable() {
		syncIndexes(t, h, "agent")
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

func TestCatalogReadDiscoversWithoutObjectRead(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init"))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "runbook/public", ""), "--repo", repo, "--command-id", "seed", "--value", `{"body":"secret procedure"}`))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "inventory.read"))

	state := asMap(t, body(t, kc(h, "show", "--as", "bot")))
	listed := businessRepositories(state)
	if len(listed) != 1 || listed[0] != repo {
		t.Fatalf("inventory.read must discover registered repositories: %#v", state)
	}
	expectCode(t, kc(h, "read", testObjectReference(t, repo, "runbook/public", ""), "--repo", repo, "--as", "bot"), "FORBIDDEN")
}

func TestDatasetSearchRejectsMissingConsumptionBeforeQuery(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init"))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "schema/runbook.body", ""), "--repo", repo, "--command-id", "schema", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string"}}}`))
	seedTextDeclaration(t, h, repo, "index/runbook-body", "schema/runbook.body", "body")
	body(t, kc(h, "put", testObjectReference(t, repo, "runbook/public", ""), "--repo", repo, "--command-id", "body", "--schema-ref", "schema/runbook.body", "--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repo))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "object.read", "--repo", repo))
	expectCode(t, kc(h, "search", "--as", "bot", "--dataset", "agent", "--query", "payment"), "FORBIDDEN")
	expectCode(t, kc(h, "search", "--as", "bot", "--repo", repo, "--query", "payment"), "USAGE_INVALID")
	body(t, kc(h, "allow", "--principal", "bot", "--action", "file.read", "--dataset", "agent"))
	if !searchAvailable() {
		expectCode(t, kc(h, "search", "--as", "bot", "--dataset", "agent", "--query", "payment"), "CAPABILITY_UNSATISFIED")
		return
	}
	syncIndexes(t, h, "agent")
	result := asMap(t, body(t, kc(h, "search", "--as", "bot", "--dataset", "agent", "--query", "payment")))
	hits := result["hits"].([]any)
	if len(hits) != 1 || asMap(t, hits[0])["objectId"] != "runbook/public" || asMap(t, hits[0])["value"] != nil {
		t.Fatalf("CLI search identity/evidence view changed: %#v", result)
	}
	read := asMap(t, body(t, kc(h, "read", testObjectReference(t, repo, "runbook/public", ""), "--as", "bot", "--dataset", "agent", "--revision", "1")))
	if asMap(t, read["value"])["body"] != "payment public procedure" {
		t.Fatalf("Dataset consumption did not deliver complete selected body: %#v", read)
	}
}

func TestHTTPWorkspaceSearchKeepsDatasetFileReadBody(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/public/runbooks"
	body(t, kc(h, "init"))
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "schema/runbook.body", ""), "--repo", repo, "--command-id", "schema", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string"}}}`))
	body(t, kc(h, "put", testObjectReference(t, repo, "runbook/public", ""), "--repo", repo, "--command-id", "body", "--schema-ref", "schema/runbook.body", "--value", `{"body":"payment public procedure"}`))
	seedTextDeclaration(t, h, repo, "index/runbook-body", "schema/runbook.body", "body")
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repo+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "taihu:alice", "--action", "file.read", "--dataset", "agent"))
	if !searchAvailable() {
		expectCode(t, kc(h, "search", "--as", "taihu:alice", "--dataset", "agent", "--query", "payment"), "CAPABILITY_UNSATISFIED")
		return
	}
	syncIndexes(t, h, "agent")

	server := httptest.NewServer(serv.HTTPHandlerWithOptions(h, serv.HTTPServerOptions{}))
	t.Cleanup(server.Close)
	if closer, ok := server.Config.Handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	raw, err := json.Marshal(map[string]any{"dataset": "agent", "query": "payment"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/object/v1/search", bytes.NewReader(raw))
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
	object := asMap(t, asMap(t, hits[0])["object"])
	if asMap(t, object["value"])["body"] != "payment public procedure" {
		t.Fatalf("Dataset file.read must deliver in-scope Canonical body: %#v", payload)
	}
	if asMap(t, object["objectRef"])["object"] == "" {
		t.Fatalf("masked hit must keep coordinates: %#v", hits[0])
	}
}

func TestObjectOnlyWorkspaceCannotCheckoutByScanning(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	group := "kr://acme/groups/payments"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	seedRepo(t, h, group)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-103", ""), "--repo", core, "--command-id", "pub", "--value", `{"body":"public"}`))
	body(t, kc(h, "put", testObjectReference(t, group, "policy/P-104", ""), "--repo", group, "--command-id", "grp", "--value", `{"body":"group"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "payments-agent", "--revision", "1",
		"--source", core+"=refs/heads/main",
		"--source", group+"=refs/heads/main"))
	body(t, kc(h, "grant", "add", "--principal", "agent:files",
		"--action", "dataset.resolve", "--dataset", "payments-agent"))

	server := httptest.NewServer(serv.HTTPHandlerWithOptions(h, serv.HTTPServerOptions{}))
	t.Cleanup(server.Close)
	if closer, ok := server.Config.Handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	raw, _ := json.Marshal(map[string]any{"dataset": "payments-agent"})
	request, err := http.NewRequest(http.MethodPost, server.URL+"/snapshot/v1/files/mounts:list", bytes.NewReader(raw))
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
		t.Fatalf("object-only workspace must not project a file tree: %s", bodyBytes)
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatal(err, string(bodyBytes))
	}
	if asMap(t, payload["error"])["code"] != "CAPABILITY_UNSATISFIED" {
		t.Fatalf("file gateway must fail closed instead of scanning objects: %#v", payload)
	}
}

func TestDatasetSearchExcludesPathsOutsidePublishedList(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy.body", ""), "--repo", core, "--command-id", "schema-body", "--path-hint", "policies/schema.yaml", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"type":"string"}}}`))
	body(t, kc(h, "put", testObjectReference(t, core, "fixture/listed", ""), "--repo", core, "--command-id", "listed", "--schema-ref", "schema/policy.body", "--path-hint", "policies/listed.yaml", "--value", `{"body":"listed payment procedure"}`))
	body(t, kc(h, "put", testObjectReference(t, core, "schema/note.body", ""), "--repo", core, "--command-id", "schema-note", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string"}}}`))
	body(t, kc(h, "put", testObjectReference(t, core, "fixture/secret", ""), "--repo", core, "--command-id", "secret", "--schema-ref", "schema/note.body", "--path-hint", "notes/secret.yaml", "--value", `{"body":"secret payment procedure"}`))
	seedIndexDeclaration(t, h, core, "index/policy-body", "text", map[string]any{"catalog": "declarative-index/opensearch@2026-10", "inputs": []any{map[string]any{"from": "schema/policy.body#/fields/body"}}, "build": map[string]any{"kind": "text"}, "uses": []any{"text"}}, "policies/index.yaml")
	body(t, kc(h, "dataset", "define", "--dataset", "docs", "--revision", "1",
		"--source", core+"=refs/heads/main@policies@policies"))
	listed := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "fixture/listed", ""), "--dataset", "docs")))
	if listed["reference"] != testObjectReference(t, core, "fixture/listed", "") {
		t.Fatalf("prefix dataset must read listed files: %#v", listed)
	}
	secret := kc(h, "read", testObjectReference(t, core, "fixture/secret", ""), "--dataset", "docs")
	expectCode(t, secret, "OBJECT_REF_UNRESOLVED")
	if strings.Contains(secret.Stdout, "secret payment procedure") {
		t.Fatal("prefix Dataset read leaked a file outside its published list", secret.Stdout)
	}

	if searchAvailable() {
		syncIndexes(t, h, "docs")
		search := asMap(t, body(t, kc(h, "search", "--dataset", "docs", "--query", "payment")))
		hits := search["hits"].([]any)
		if len(hits) != 1 || asMap(t, hits[0])["objectId"] != "fixture/listed" {
			t.Fatalf("SEARCH must not return paths outside the serving file list: %#v", search)
		}
	} else {
		expectCode(t, kc(h, "search", "--dataset", "docs", "--query", "payment"), "CAPABILITY_UNSATISFIED")
	}
}

func TestHistoricalDatasetReadStaysOnPublishedCommit(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy.body", ""), "--repo", core, "--command-id", "schema-body", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"type":"string"}}}`))
	seedTextDeclaration(t, h, core, "index/policy-body", "schema/policy.body", "body")
	frozenCommit := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "v1", "--schema-ref", "schema/policy.body", "--value", `{"body":"frozen payment"}`)))["result"])["newCommit"].(string)
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	if searchAvailable() {
		syncIndexes(t, h, "agent")
	}
	laterCommit := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--repo", core, "--command-id", "v2", "--schema-ref", "schema/policy.body", "--value", `{"body":"later live payment"}`)))["result"])["newCommit"].(string)
	if searchAvailable() {
		syncIndexes(t, h, "agent")
	}
	frozen := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/A", ""), "--dataset", "agent")))
	if frozen["reference"] != testObjectReference(t, core, "policy/A", "") || asMap(t, asMap(t, frozen)["value"])["body"] != "frozen payment" {
		t.Fatalf("published dataset READ must stay on Snapshot: %#v", frozen)
	}
	if asMap(t, frozen)["commit"] != frozenCommit {
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
