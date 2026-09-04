package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kc/cli"
	"kc/internal/testkit"
)

func TestLocalCLISearchDoesNotCatchUpProjection(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", "--command-id", "i1", "--repo", core, "--object", "policy/A", "--value", `{"body":"needs a runbook"}`))
	expectCode(t, kc(h, "search", "--repo", core, "--query", "runbook"), "CAPABILITY_UNSATISFIED")
}

func TestServeProjectionWorkerCatchesCommitWithoutSync(t *testing.T) {
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required")
	}
	home := testkit.TempDir(t)
	catalogID := "kr://acme/product"
	repositoryID := "kr://acme/public/core"
	workspaceID := "oncall"
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

	body(t, governor("admin", "grant", "add", "--principal", provider,
		"--action", "writer.commit,writer.preview,knowledge.read",
		"--repo", repositoryID))
	drafts := writeProviderDrafts(t)
	changeset := filepath.Join(t.TempDir(), "changeset.json")
	body(t, asProvider("pack", "--repo", repositoryID, "--dir", drafts, "--out", changeset))
	published := asMap(t, body(t, asProvider("writer", "commit", "--command-id", "source-1", "--changeset", changeset)))
	commit := publishedCommit(t, published)

	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "catalog.read,workspace.resolve,workspace.consume", "--catalog", catalogID))
	body(t, governor("admin", "grant", "add", "--principal", consumer,
		"--action", "knowledge.read,knowledge.search,knowledge.schema.read",
		"--repo", repositoryID))
	body(t, governor("workspace", "define", "--workspace", workspaceID, "--revision", "1",
		"--source", repositoryID))

	pin := asMap(t, body(t, asConsumer("workspace", "pin", "--workspace", workspaceID)))
	if asMap(t, pin["repositories"])[repositoryID] != commit {
		t.Fatalf("pin %#v", pin)
	}
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	search := waitRemoteSearchHits(t, func() kcRunResult {
		return asConsumer("knowledge", "search", "--workspace", workspaceID, "--pin", string(pinJSON), "--query", "冻结窗口")
	}, 1)
	if search["completeness"] != "complete" {
		t.Fatalf("search %#v", search)
	}
}

func waitRemoteSearchHits(t *testing.T, run func() kcRunResult, want int) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last kcRunResult
	for time.Now().Before(deadline) {
		last = run()
		if last.Status == 0 {
			var payload map[string]any
			if err := json.Unmarshal([]byte(last.Stdout), &payload); err != nil {
				t.Fatal(err, last.Stdout)
			}
			if hits, _ := payload["hits"].([]any); len(hits) == want {
				return payload
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("search did not become ready without projection sync: status=%d stdout=%s", last.Status, last.Stdout)
	return nil
}
