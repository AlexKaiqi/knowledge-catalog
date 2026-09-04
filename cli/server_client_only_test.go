package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestLocalDeploymentBootstrapsThenUsesServerClientBoundary(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/local-server"
	principal := "agent:local-admin"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	expectCode(t, kc(home, "local", "grant", "bootstrap"), "USAGE_INVALID")
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", principal))
	expectCode(t, kc(home, "local", "grant", "bootstrap", "--principal", "agent:other"), "PRECONDITION_FAILED")

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	result := kcRemote(t, server.URL, principal, "catalog", "show", "--catalog", catalogID)
	state := asMap(t, body(t, result))
	if state["catalogId"] != catalogID {
		t.Fatalf("server returned wrong catalog: %#v", state)
	}
}

// TestRemoteProviderReadBackAndConsumerDiscovery is the product Client CLI
// journey in three phases. Host bootstrap stays in the harness. The governor
// authorizes, composes, and maintains search. The provider only knows the
// Server, their identity, their knowledge source id and their drafts. The
// consumer only knows the Server, their identity, and the question they want
// answered. Consumers discover a knowledge set only after it has been composed.
func TestRemoteProviderReadBackAndConsumerDiscovery(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/product"
	repositoryID := "kr://acme/public/core"
	workspaceID := "oncall"
	admin := "agent:local-admin"
	provider := "agent:provider"
	consumer := "agent:consumer"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	governor := func(args ...string) kcRunResult {
		return kcRemote(t, server.URL, admin, args...)
	}
	asProvider := func(args ...string) kcRunResult {
		assertProductArgs(t, args)
		return kcRemote(t, server.URL, provider, args...)
	}
	asConsumer := func(args ...string) kcRunResult {
		assertProductArgs(t, args)
		return kcRemote(t, server.URL, consumer, args...)
	}

	// 1. Authorize the provider before they write.
	body(t, governor("admin", "grant", "add", "--principal", provider,
		"--action", "writer.commit,writer.preview,knowledge.read,knowledge.provenance,knowledge.history.read,knowledge.schema.read",
		"--repo", repositoryID))

	// 2. Provider publishes, then reads back on the same --repo. ingest must
	// not move HEAD; commit must. A knowledge set is not a write prerequisite.
	who := asMap(t, body(t, asProvider("whoami")))
	if who["principal"] != provider {
		t.Fatalf("provider whoami: %#v", who)
	}
	assertNoHostLeak(t, home, who)

	headBefore := asMap(t, body(t, asProvider("writer", "head", "--repo", repositoryID)))
	assertInventoryJSON(t, home, headBefore)
	if headBefore["repository"] != repositoryID || headBefore["commit"] == "" {
		t.Fatalf("provider head before publish: %#v", headBefore)
	}

	drafts := writeProviderDrafts(t)
	changeset := filepath.Join(t.TempDir(), "changeset.json")
	preview := asMap(t, body(t, asProvider("pack", "--repo", repositoryID, "--dir", drafts, "--out", changeset)))
	assertInventoryJSON(t, home, preview)
	if _, ok := preview["changeSet"]; ok {
		t.Fatalf("ingest --out must keep the ChangeSet in the file, not stdout: %#v", preview)
	}
	if asMap(t, preview["diagnostics"])["files"] != float64(2) {
		t.Fatalf("provider ingest must preview the drafts they already have: %#v", preview)
	}
	headAfterIngest := asMap(t, body(t, asProvider("writer", "head", "--repo", repositoryID)))
	if headAfterIngest["commit"] != headBefore["commit"] {
		t.Fatalf("ingest must not publish: before %#v after %#v", headBefore, headAfterIngest)
	}

	published := asMap(t, body(t, asProvider("writer", "commit", "--command-id", "source-1", "--changeset", changeset)))
	commit := publishedCommit(t, published)
	head := asMap(t, body(t, asProvider("writer", "head", "--repo", repositoryID)))
	if head["repository"] != repositoryID || head["commit"] != commit {
		t.Fatalf("provider head must return the published commit: %#v", head)
	}
	if head["commit"] == headBefore["commit"] {
		t.Fatalf("commit must publish a new version: %#v", head)
	}
	assertInventoryJSON(t, home, head)

	providerObject := "runbook/payment-oncall"
	direct := asMap(t, body(t, asProvider("knowledge", "read", "--repo", repositoryID, "--object", providerObject)))
	if direct["commit"] != commit || asMap(t, direct["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("provider could not read back over the product Client: %#v", direct)
	}
	assertNoHostLeak(t, home, direct)
	providerProvenance := asMap(t, body(t, asProvider("knowledge", "provenance", "--repo", repositoryID, "--object", providerObject)))
	if providerProvenance["commit"] != commit {
		t.Fatalf("provider provenance: %#v", providerProvenance)
	}
	providerResolved := asMap(t, body(t, asProvider("knowledge", "resolve", "--repo", repositoryID, "--object", providerObject)))
	if providerResolved["status"] != "RESOLVED" || providerResolved["commit"] != commit {
		t.Fatalf("provider knowledge resolve: %#v", providerResolved)
	}
	published2 := asMap(t, body(t, asProvider("writer", "put", "--command-id", "source-2", "--repo", repositoryID,
		"--object", providerObject, "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"切换支付流量前先检查冻结窗口，并核对灰度"}`)))
	commit = publishedCommit(t, published2)
	expectCode(t, asProvider("workspace", "define", "--workspace", workspaceID,
		"--revision", "1", "--source", repositoryID), "FORBIDDEN")

	// 3. Governor names the knowledge set, authorizes the consumer, and
	// prepares SEARCH. Consumers must not see a named set before this.
	beforeCompose := asMap(t, body(t, governor("catalog", "show")))
	if _, found := knowledgeSetFromInventory(beforeCompose, workspaceID); found {
		t.Fatalf("named knowledge set must not exist before compose: %#v", beforeCompose)
	}

	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "catalog.read,workspace.resolve,workspace.consume", "--catalog", catalogID))
	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "knowledge.read,knowledge.provenance,knowledge.search,knowledge.schema.read,knowledge.history.read",
		"--repo", repositoryID))
	body(t, governor("workspace", "define", "--workspace", workspaceID, "--revision", "1",
		"--source", repositoryID))
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) != "" {
		body(t, governor("operations", "projection", "sync", "--repo", repositoryID))
	}

	// 4. Consumer discovers the composed set, freezes it, then searches or
	// reads. Object ids come from SEARCH hits, never from guessing.
	consumerWho := asMap(t, body(t, asConsumer("whoami")))
	if consumerWho["principal"] != consumer {
		t.Fatalf("consumer whoami: %#v", consumerWho)
	}
	expectCode(t, asConsumer("writer", "put", "--command-id", "consumer-write", "--repo", repositoryID,
		"--object", "guessed/object", "--value", `{"body":"tamper"}`), "FORBIDDEN")

	inventory := asMap(t, body(t, asConsumer("catalog", "list")))
	listed := inventory["catalogs"].([]any)
	if len(listed) != 1 || asMap(t, listed[0])["id"] != catalogID {
		t.Fatalf("consumer could not discover Catalogs: %#v", inventory)
	}
	assertInventoryJSON(t, home, inventory)

	state := asMap(t, body(t, asConsumer("catalog", "show")))
	if state["catalogId"] != catalogID {
		t.Fatalf("consumer catalog show did not infer the only visible Catalog: %#v", state)
	}
	assertInventoryJSON(t, home, state)
	discoveredWorkspace, discoveredRepo := discoveredKnowledgeSet(t, state, repositoryID)
	if discoveredWorkspace != workspaceID {
		t.Fatalf("consumer discovered the wrong knowledge set: %s", discoveredWorkspace)
	}

	listedSets := asMap(t, body(t, asConsumer("workspace", "list")))
	assertInventoryJSON(t, home, listedSets)
	shown := asMap(t, body(t, asConsumer("workspace", "show", "--workspace", discoveredWorkspace)))
	assertInventoryJSON(t, home, shown)
	if shown["workspaceId"] != discoveredWorkspace {
		t.Fatalf("workspace show: %#v", shown)
	}

	schemas := asMap(t, body(t, asConsumer("knowledge", "schema", "list", "--repo", discoveredRepo)))
	if schemas["repository"] != discoveredRepo || schemas["exhausted"] != true {
		t.Fatalf("consumer schema browse must use the discovered source: %#v", schemas)
	}
	assertNoHostLeak(t, home, schemas)
	if len(schemas["schemas"].([]any)) == 0 {
		t.Fatalf("consumer schema browse returned no schemas: %#v", schemas)
	}

	pin := asMap(t, body(t, asConsumer("workspace", "pin", "--workspace", discoveredWorkspace)))
	if asMap(t, pin["repositories"])[discoveredRepo] != commit {
		t.Fatalf("named knowledge set did not freeze the published commit: %#v", pin)
	}
	assertInventoryJSON(t, home, pin)
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	expectCode(t, asConsumer("workspace", "pin", "--workspace", discoveredWorkspace,
		"--object", providerObject), "USAGE_INVALID")
	resolvedObject := body(t, asConsumer("knowledge", "resolve", "--workspace", discoveredWorkspace,
		"--pin", string(pinJSON), "--object", providerObject)).([]any)
	if len(resolvedObject) != 1 || asMap(t, resolvedObject[0])["status"] != "RESOLVED" || asMap(t, resolvedObject[0])["commit"] != commit {
		t.Fatalf("consumer knowledge resolve: %#v", resolvedObject)
	}
	assertNoHostLeak(t, home, resolvedObject)
	absent := body(t, asConsumer("knowledge", "resolve", "--workspace", discoveredWorkspace,
		"--pin", string(pinJSON), "--object", "missing/nope")).([]any)
	if len(absent) != 0 {
		t.Fatalf("consumer resolve of a missing object must be an empty union: %#v", absent)
	}
	expectCode(t, asConsumer("knowledge", "resolve", "--workspace", discoveredWorkspace,
		"--pin", string(pinJSON), "--object", providerObject, "--member", "user:bob"), "USAGE_INVALID")
	history := asMap(t, body(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--limit", "1")))
	if history["exhausted"] == true || history["continuation"] == "" {
		t.Fatalf("consumer log must page introducing commits: %#v", history)
	}
	assertNoHostLeak(t, home, history)
	nextPage := asMap(t, body(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--limit", "1", "--continuation", history["continuation"].(string))))
	if len(asMap(t, nextPage["logs"].([]any)[0])["revisions"].([]any)) == 0 {
		t.Fatalf("consumer log continuation: %#v", nextPage)
	}
	zeroLog := asMap(t, body(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--limit", "0")))
	if zeroLog["exhausted"] != true || len(asMap(t, zeroLog["logs"].([]any)[0])["revisions"].([]any)) < 2 {
		t.Fatalf("--limit 0 must mean the default consumer log page: %#v", zeroLog)
	}
	expectCode(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--limit", "201"), "USAGE_INVALID")
	expectCode(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, asConsumer("knowledge", "log", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
		"--object", providerObject, "--member", "user:bob"), "USAGE_INVALID")
	auditZero := asMap(t, body(t, governor("catalog", "audit", "--catalog", catalogID, "--limit", "0")))
	auditDefault := asMap(t, body(t, governor("catalog", "audit", "--catalog", catalogID)))
	if len(auditZero["entries"].([]any)) != len(auditDefault["entries"].([]any)) {
		t.Fatalf("remote audit --limit 0 must mean the default page: %#v vs %#v", auditZero, auditDefault)
	}

	searchArgs := []string{"knowledge", "search", "--workspace", discoveredWorkspace, "--pin", string(pinJSON), "--query", "冻结窗口"}
	searchResult := asConsumer(searchArgs...)
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		expectCode(t, searchResult, "CAPABILITY_UNSATISFIED")
		assertNoOperatorHint(t, searchResult)
	} else {
		search := asMap(t, body(t, searchResult))
		assertNoHostLeak(t, home, search)
		if search["completeness"] != "complete" || len(search["hits"].([]any)) != 1 {
			t.Fatalf("consumer search was not complete at the pin: %#v", search)
		}
		objectID := searchHitObjectID(t, search)
		values := body(t, asConsumer("knowledge", "read", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
			"--object", objectID)).([]any)
		if len(values) != 1 || asMap(t, values[0])["commit"] != commit {
			t.Fatalf("consumer read did not reuse the pin: %#v", values)
		}
		if asMap(t, asMap(t, values[0])["value"])["body"] != "切换支付流量前先检查冻结窗口，并核对灰度" {
			t.Fatalf("consumer read value: %#v", values)
		}
		assertNoHostLeak(t, home, values)
		provenance := body(t, asConsumer("knowledge", "provenance", "--workspace", discoveredWorkspace, "--pin", string(pinJSON),
			"--object", objectID)).([]any)
		if len(provenance) != 1 || asMap(t, provenance[0])["commit"] != commit {
			t.Fatalf("consumer provenance did not reuse the pin: %#v", provenance)
		}
	}

	adhoc := asMap(t, body(t, asConsumer("workspace", "pin", "--source", discoveredRepo)))
	if asMap(t, adhoc["repositories"])[discoveredRepo] != commit || adhoc["pinId"] == "" {
		t.Fatalf("temporary knowledge set resolve failed: %#v", adhoc)
	}
	assertInventoryJSON(t, home, adhoc)
}

func writeProviderDrafts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schema := "---\nobject_id: schema/runbook.body\n---\n" +
		`{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}` + "\n"
	object := "---\nobject_id: runbook/payment-oncall\nschema_ref: schema/runbook.body\nkind: Entity\n" +
		`provenance: {"originKind":"SOURCE","sourceRefs":["file:///source/runbooks/payment-oncall.md"]}` +
		"\n---\n{\"body\":\"切换支付流量前先检查冻结窗口\"}\n"
	if err := os.WriteFile(filepath.Join(dir, "schema.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runbook.json"), []byte(object), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func discoveredKnowledgeSet(t *testing.T, state map[string]any, wantRepo string) (workspaceID, repositoryID string) {
	t.Helper()
	for _, raw := range state["workspaces"].([]any) {
		workspace := asMap(t, raw)
		id, _ := workspace["workspaceId"].(string)
		if id == "" {
			continue
		}
		for _, repo := range workspace["repositories"].([]any) {
			if repo == wantRepo {
				return id, wantRepo
			}
		}
	}
	t.Fatalf("consumer inventory did not include the published knowledge source: %#v", state)
	return "", ""
}

func knowledgeSetFromInventory(state map[string]any, workspaceID string) (map[string]any, bool) {
	raw, _ := state["workspaces"].([]any)
	for _, item := range raw {
		workspace, _ := item.(map[string]any)
		if workspace["workspaceId"] == workspaceID {
			return workspace, true
		}
	}
	return nil, false
}

func searchHitObjectID(t *testing.T, search map[string]any) string {
	t.Helper()
	hit := asMap(t, search["hits"].([]any)[0])
	if version, ok := hit["version"].(map[string]any); ok {
		if objectID, _ := version["objectId"].(string); objectID != "" {
			return objectID
		}
	}
	address := asMap(t, asMap(t, hit["knowledge"])["address"])
	objectID, _ := address["objectId"].(string)
	if objectID == "" {
		t.Fatalf("search hit has no object id: %#v", hit)
	}
	return objectID
}

func assertProductArgs(t *testing.T, args []string) {
	t.Helper()
	for _, arg := range args {
		if arg == "--home" || strings.HasPrefix(arg, "--home=") {
			t.Fatalf("product role must not open --home: %v", args)
		}
		if arg == "--ref" || strings.HasPrefix(arg, "--ref=") || strings.Contains(arg, "refs/heads") {
			t.Fatalf("product role must not name snapshot refs: %v", args)
		}
	}
}

func assertNoHostLeak(t *testing.T, home string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if home != "" && strings.Contains(text, home) {
		t.Fatalf("product JSON leaked host path %s: %s", home, text)
	}
}

func assertInventoryJSON(t *testing.T, home string, payload any) {
	t.Helper()
	assertNoHostLeak(t, home, payload)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, leak := range []string{`"dir"`, `"selector"`, `"baseRev"`, "refs/heads"} {
		if strings.Contains(text, leak) {
			t.Fatalf("inventory leaked %s: %s", leak, text)
		}
	}
}

func assertNoOperatorHint(t *testing.T, result kcRunResult) {
	t.Helper()
	text := result.Stdout
	for _, leak := range []string{"operations projection", "operations access", "kc operations", "run operations", "OpenSearch", "Dolt", "Gitea", "--index", "refs/heads", "--home"} {
		if strings.Contains(text, leak) {
			t.Fatalf("consumer error taught operator internals %q: %s", leak, text)
		}
	}
}

func publishedCommit(t *testing.T, payload map[string]any) string {
	t.Helper()
	if result, ok := payload["result"].(map[string]any); ok {
		if commit, _ := result["newCommit"].(string); commit != "" {
			return commit
		}
	}
	if commit, _ := payload["newCommit"].(string); commit != "" {
		return commit
	}
	t.Fatalf("writer receipt has no newCommit: %#v", payload)
	return ""
}
