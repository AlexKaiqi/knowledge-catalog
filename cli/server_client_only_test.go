package cli_test

import (
	"encoding/json"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDeploymentBootstrapsThenUsesServerClientBoundary(t *testing.T) {
	home := testkit.TempDir(t)
	principal := "agent:local-admin"
	body(t, kc(home, "local", "init"))
	expectCode(t, kc(home, "local", "grant", "bootstrap"), "USAGE_INVALID")
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", principal))
	expectCode(t, kc(home, "local", "grant", "bootstrap", "--principal", "agent:other"), "PRECONDITION_FAILED")

	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	state := asMap(t, body(t, kcRemote(t, server.URL, principal, "show")))
	if _, present := state["catalogId"]; present {
		t.Fatalf("server returned wrong catalog: %#v", state)
	}
}

// TestRemoteProviderReadBackAndConsumerDiscovery is the product Client CLI
// journey in three phases. Host bootstrap stays in the harness. The governor
// authorizes, composes, and maintains search. The provider only knows the
// Server, their identity, their object source id and their drafts. The
// consumer only knows the Server, their identity, and the question they want
// answered. Consumers discover a dataset only after it has been composed.
func TestRemoteProviderReadBackAndConsumerDiscovery(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/core"
	setID := "oncall"
	admin := "agent:local-admin"
	provider := "agent:provider"
	consumer := "agent:consumer"
	body(t, kc(home, "local", "init"))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))

	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
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
	body(t, governor("grant", "add", "--principal", provider,
		"--action", "writer.commit,writer.preview,object.read,object.provenance,object.history.read,object.schema.read",
		"--repo", repositoryID))

	// 2. Provider publishes, then reads back on the same --repo. ingest must
	// not move HEAD; commit must. A dataset is not a write prerequisite.
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
	published := asMap(t, body(t, asProvider("writer", "commit", "--command-id", "source-1", "--repo", repositoryID, "--dir", drafts)))
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
	direct := asMap(t, body(t, asProvider("read", testObjectReference(t, repositoryID, providerObject, ""))))
	if direct["commit"] != commit || asMap(t, direct["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("provider could not read back over the product Client: %#v", direct)
	}
	assertNoHostLeak(t, home, direct)
	providerProvenance := asMap(t, body(t, asProvider("provenance", "--repo", repositoryID, "--object", providerObject)))
	if providerProvenance["commit"] != commit {
		t.Fatalf("provider provenance: %#v", providerProvenance)
	}
	providerResolved := asMap(t, body(t, asProvider("resolve", "--repo", repositoryID, "--object", providerObject)))
	if providerResolved["status"] != "RESOLVED" || providerResolved["commit"] != commit {
		t.Fatalf("provider resolve: %#v", providerResolved)
	}
	published2 := asMap(t, body(t, asProvider("writer", "put", testObjectReference(t, repositoryID, providerObject, ""), "--command-id", "source-2", "--schema-ref", "schema/runbook.body", "--value", `{"body":"切换支付流量前先检查冻结窗口，并核对灰度"}`)))
	commit = publishedCommit(t, published2)
	expectCode(t, asProvider("dataset", "define", "--dataset", setID,
		"--revision", "1", "--source", repositoryID), "FORBIDDEN")

	// 3. Governor names the dataset, authorizes the consumer, and
	// prepares SEARCH. Consumers must not see a named set before this.
	beforeCompose := asMap(t, body(t, governor("show")))
	if _, found := datasetFromInventory(beforeCompose, setID); found {
		t.Fatalf("named dataset must not exist before compose: %#v", beforeCompose)
	}

	body(t, governor("grant", "add", "--principal", consumer,
		"--action", "inventory.read,dataset.resolve,file.read"))
	body(t, governor("grant", "add", "--principal", consumer,
		"--action", "object.read,object.provenance,object.search,object.schema.read,object.history.read",
		"--repo", repositoryID))
	body(t, governor("dataset", "define", "--dataset", setID, "--revision", "1",
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
	expectCode(t, asConsumer("writer", "put", testObjectReference(t, repositoryID, "guessed/object", ""), "--command-id", "consumer-write", "--value", `{"body":"tamper"}`), "FORBIDDEN")

	expectCode(t, asConsumer("catalog", "list"), "USAGE_INVALID")
	state := asMap(t, body(t, asConsumer("show")))
	if _, present := state["catalogId"]; present {
		t.Fatalf("consumer show did not infer the only visible Catalog: %#v", state)
	}
	assertInventoryJSON(t, home, state)
	discoveredWorkspace, discoveredRepo := discoveredDataset(t, state, repositoryID)
	if discoveredWorkspace != setID {
		t.Fatalf("consumer discovered the wrong dataset: %s", discoveredWorkspace)
	}
	if workspace, ok := datasetFromInventory(state, discoveredWorkspace); !ok || workspace["id"] != discoveredWorkspace {
		t.Fatalf("show must include the named dataset: %#v", state)
	}

	schemas := asMap(t, body(t, asConsumer("schema", "list", "--repo", discoveredRepo)))
	if schemas["repository"] != discoveredRepo || schemas["exhausted"] != nil {
		t.Fatalf("consumer schema browse must use the discovered source: %#v", schemas)
	}
	assertNoHostLeak(t, home, schemas)
	if len(schemas["schemas"].([]any)) == 0 {
		t.Fatalf("consumer schema browse returned no schemas: %#v", schemas)
	}

	resolvedObject := body(t, asConsumer("resolve",
		"--dataset", discoveredWorkspace, "--object", providerObject)).([]any)
	if len(resolvedObject) != 1 || asMap(t, resolvedObject[0])["status"] != "RESOLVED" || asMap(t, resolvedObject[0])["commit"] != commit {
		t.Fatalf("consumer resolve: %#v", resolvedObject)
	}
	assertNoHostLeak(t, home, resolvedObject)
	absent := body(t, asConsumer("resolve",
		"--dataset", discoveredWorkspace, "--object", "missing/nope")).([]any)
	if len(absent) != 0 {
		t.Fatalf("consumer resolve of a missing object must be an empty union: %#v", absent)
	}
	expectCode(t, asConsumer("resolve",
		"--dataset", discoveredWorkspace, "--object", providerObject, "--member", "user:bob"), "USAGE_INVALID")
	history := asMap(t, body(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--limit", "1")))
	if history["exhausted"] != nil || history["continuation"] == "" {
		t.Fatalf("consumer log must page introducing commits: %#v", history)
	}
	assertNoHostLeak(t, home, history)
	nextPage := asMap(t, body(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--limit", "1", "--continuation", history["continuation"].(string))))
	if len(asMap(t, nextPage["logs"].([]any)[0])["revisions"].([]any)) == 0 {
		t.Fatalf("consumer log continuation: %#v", nextPage)
	}
	zeroLog := asMap(t, body(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--limit", "0")))
	if zeroLog["exhausted"] != nil || len(asMap(t, zeroLog["logs"].([]any)[0])["revisions"].([]any)) < 2 {
		t.Fatalf("--limit 0 must mean the default consumer log page: %#v", zeroLog)
	}
	expectCode(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--limit", "201"), "USAGE_INVALID")
	expectCode(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--aspect", "io"), "USAGE_INVALID")
	expectCode(t, asConsumer("log", "--dataset", discoveredWorkspace,
		"--object", providerObject, "--member", "user:bob"), "USAGE_INVALID")
	auditZero := asMap(t, body(t, governor("registry", "audit", "--limit", "0")))
	auditDefault := asMap(t, body(t, governor("registry", "audit")))
	if len(auditZero["entries"].([]any)) != len(auditDefault["entries"].([]any)) {
		t.Fatalf("remote audit --limit 0 must mean the default page: %#v vs %#v", auditZero, auditDefault)
	}

	searchArgs := []string{"search", "--dataset", discoveredWorkspace, "--query", "冻结窗口"}
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
		values := asMap(t, body(t, asConsumer("read", testObjectReference(t, repositoryID, objectID, ""), "--dataset", discoveredWorkspace)))
		if values["reference"] != testObjectReference(t, repositoryID, objectID, "") || asMap(t, values)["commit"] != commit {
			t.Fatalf("consumer read did not reuse the pin: %#v", values)
		}
		if asMap(t, asMap(t, values)["value"])["body"] != "切换支付流量前先检查冻结窗口，并核对灰度" {
			t.Fatalf("consumer read value: %#v", values)
		}
		assertNoHostLeak(t, home, values)
		provenance := body(t, asConsumer("provenance", "--dataset", discoveredWorkspace,
			"--object", objectID)).([]any)
		if len(provenance) != 1 || asMap(t, provenance[0])["commit"] != commit {
			t.Fatalf("consumer provenance did not reuse the pin: %#v", provenance)
		}
	}

	// A Repository READ returns one value; only Dataset reads return a union.
	value := asMap(t, body(t, asConsumer("read", testObjectReference(t, discoveredRepo, providerObject, ""), "--commit", commit)))
	if value["commit"] != commit || asMap(t, value["value"])["body"] != "切换支付流量前先检查冻结窗口，并核对灰度" {
		t.Fatalf("exact --repo --commit replay cannot be consumed: %#v", value)
	}
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

func discoveredDataset(t *testing.T, state map[string]any, wantRepo string) (setID, repositoryID string) {
	t.Helper()
	for _, raw := range state["datasets"].([]any) {
		workspace := asMap(t, raw)
		id, _ := workspace["id"].(string)
		if id == "" {
			continue
		}
		for _, repo := range workspace["repositories"].([]any) {
			if repo == wantRepo {
				return id, wantRepo
			}
		}
	}
	t.Fatalf("consumer inventory did not include the published object source: %#v", state)
	return "", ""
}

func datasetFromInventory(state map[string]any, setID string) (map[string]any, bool) {
	raw, _ := state["datasets"].([]any)
	for _, item := range raw {
		workspace, _ := item.(map[string]any)
		if workspace["id"] == setID {
			return workspace, true
		}
	}
	return nil, false
}

func searchHitObjectID(t *testing.T, search map[string]any) string {
	t.Helper()
	hit := asMap(t, search["hits"].([]any)[0])
	if objectID, _ := hit["objectId"].(string); objectID != "" {
		return objectID
	}
	if version, ok := hit["version"].(map[string]any); ok {
		if objectID, _ := version["objectId"].(string); objectID != "" {
			return objectID
		}
	}
	address := asMap(t, asMap(t, hit["object"])["address"])
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
	for _, leak := range []string{"operations projection", "operations access", "kc operations", "run operations", "OpenSearch", "Gitea", "--index", "refs/heads", "--home"} {
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
