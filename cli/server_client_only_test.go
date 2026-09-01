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

	result := kcRemote(server.URL, principal, "catalog", "show", "--catalog", catalogID)
	state := asMap(t, body(t, result))
	if state["catalogId"] != catalogID {
		t.Fatalf("server returned wrong catalog: %#v", state)
	}
}

// TestRemoteProviderReadBackAndConsumerDiscovery is the product Client CLI
// journey. Host bootstrap, grants, composition and projection sync stay in
// the harness. The provider only knows the Server, their identity, their
// knowledge source id and their drafts. The consumer only knows the Server,
// their identity, and the question they want answered.
func TestRemoteProviderReadBackAndConsumerDiscovery(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/product"
	repositoryID := "kr://acme/public/core"
	admin := "agent:local-admin"
	provider := "agent:provider"
	consumer := "agent:consumer"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	governor := func(args ...string) kcRunResult {
		return kcRemote(server.URL, admin, args...)
	}
	asProvider := func(args ...string) kcRunResult {
		assertProductArgs(t, args)
		return kcRemote(server.URL, provider, args...)
	}
	asConsumer := func(args ...string) kcRunResult {
		assertProductArgs(t, args)
		return kcRemote(server.URL, consumer, args...)
	}

	body(t, governor("admin", "grant", "add", "--principal", provider,
		"--action", "writer.commit,writer.preview,knowledge.read,knowledge.provenance,knowledge.history.read,knowledge.schema.read",
		"--repo", repositoryID))
	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "catalog.read,workspace.resolve,workspace.consume", "--catalog", catalogID))
	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "knowledge.read,knowledge.provenance,knowledge.search,knowledge.schema.read,knowledge.history.read",
		"--repo", repositoryID))

	who := asMap(t, body(t, asProvider("identity", "whoami")))
	if who["principal"] != provider {
		t.Fatalf("provider whoami: %#v", who)
	}
	assertNoHostLeak(t, home, who)

	drafts := writeProviderDrafts(t)
	changeset := filepath.Join(t.TempDir(), "changeset.json")
	preview := asMap(t, body(t, asProvider("writer", "ingest", "--repo", repositoryID, "--dir", drafts, "--out", changeset)))
	assertNoHostLeak(t, home, preview)
	if asMap(t, preview["diagnostics"])["files"] != float64(2) {
		t.Fatalf("provider ingest must preview the drafts they already have: %#v", preview)
	}
	published := asMap(t, body(t, asProvider("writer", "commit", "--command-id", "source-1", "--changeset", changeset)))
	commit := publishedCommit(t, published)
	head := asMap(t, body(t, asProvider("writer", "head", "--repo", repositoryID)))
	if head["repository"] != repositoryID || head["commit"] != commit {
		t.Fatalf("provider head must return the published commit: %#v", head)
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
	expectCode(t, asProvider("catalog", "workspace", "define", "--workspace", "oncall",
		"--revision", "1", "--source", repositoryID), "FORBIDDEN")

	expectCode(t, asConsumer("writer", "put", "--command-id", "consumer-write", "--repo", repositoryID,
		"--object", "guessed/object", "--value", `{"body":"tamper"}`), "FORBIDDEN")

	consumerWho := asMap(t, body(t, asConsumer("identity", "whoami")))
	if consumerWho["principal"] != consumer {
		t.Fatalf("consumer whoami: %#v", consumerWho)
	}

	inventory := asMap(t, body(t, asConsumer("catalog", "list")))
	listed := inventory["catalogs"].([]any)
	if len(listed) != 1 || asMap(t, listed[0])["id"] != catalogID {
		t.Fatalf("consumer could not discover Catalogs: %#v", inventory)
	}
	assertInventoryJSON(t, home, inventory)

	body(t, governor("catalog", "workspace", "define", "--workspace", "oncall", "--revision", "1",
		"--source", repositoryID))

	state := asMap(t, body(t, asConsumer("catalog", "show")))
	if state["catalogId"] != catalogID {
		t.Fatalf("consumer catalog show did not infer the only visible Catalog: %#v", state)
	}
	assertInventoryJSON(t, home, state)
	workspaceID, discoveredRepo := discoveredKnowledgeSet(t, state, repositoryID)

	listedSets := asMap(t, body(t, asConsumer("catalog", "workspace", "list")))
	assertInventoryJSON(t, home, listedSets)
	shown := asMap(t, body(t, asConsumer("catalog", "workspace", "show", "--workspace", workspaceID)))
	assertInventoryJSON(t, home, shown)
	if shown["workspaceId"] != workspaceID {
		t.Fatalf("workspace show: %#v", shown)
	}

	schemas := asMap(t, body(t, asConsumer("knowledge", "schema", "browse", "--repo", discoveredRepo)))
	if schemas["repository"] != discoveredRepo || schemas["exhausted"] != true {
		t.Fatalf("consumer schema browse must use the discovered source: %#v", schemas)
	}
	assertNoHostLeak(t, home, schemas)
	if len(schemas["schemas"].([]any)) == 0 {
		t.Fatalf("consumer schema browse returned no schemas: %#v", schemas)
	}

	pin := asMap(t, body(t, asConsumer("catalog", "workspace", "resolve", "--workspace", workspaceID)))
	if asMap(t, pin["repositories"])[discoveredRepo] != commit {
		t.Fatalf("named knowledge set did not freeze the published commit: %#v", pin)
	}
	assertInventoryJSON(t, home, pin)
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}

	searchArgs := []string{"knowledge", "search", "--workspace", workspaceID, "--pin", string(pinJSON), "--query", "冻结窗口"}
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		expectCode(t, asConsumer(searchArgs...), "CAPABILITY_UNSATISFIED")
	} else {
		body(t, governor("operations", "projection", "sync", "--repo", discoveredRepo))
		search := asMap(t, body(t, asConsumer(searchArgs...)))
		assertNoHostLeak(t, home, search)
		if search["completeness"] != "complete" || len(search["hits"].([]any)) != 1 {
			t.Fatalf("consumer search was not complete at the pin: %#v", search)
		}
		objectID := searchHitObjectID(t, search)
		values := body(t, asConsumer("knowledge", "read", "--workspace", workspaceID, "--pin", string(pinJSON),
			"--object", objectID)).([]any)
		if len(values) != 1 || asMap(t, values[0])["commit"] != commit {
			t.Fatalf("consumer read did not reuse the pin: %#v", values)
		}
		if asMap(t, asMap(t, values[0])["value"])["body"] != "切换支付流量前先检查冻结窗口" {
			t.Fatalf("consumer read value: %#v", values)
		}
		assertNoHostLeak(t, home, values)
		provenance := body(t, asConsumer("knowledge", "provenance", "--workspace", workspaceID, "--pin", string(pinJSON),
			"--object", objectID)).([]any)
		if len(provenance) != 1 || asMap(t, provenance[0])["commit"] != commit {
			t.Fatalf("consumer provenance did not reuse the pin: %#v", provenance)
		}
		history := body(t, asConsumer("knowledge", "log", "--workspace", workspaceID, "--pin", string(pinJSON),
			"--object", objectID)).([]any)
		if len(history) == 0 {
			t.Fatalf("consumer log was empty: %#v", history)
		}
		assertNoHostLeak(t, home, history)
	}

	adhoc := asMap(t, body(t, asConsumer("catalog", "workspace", "resolve", "--source", discoveredRepo)))
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
