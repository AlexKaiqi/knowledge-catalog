package cli_test

import (
	"encoding/json"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalCLISearchDoesNotCatchUpProjection(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init"))
	// This contract is about the no-provider local profile; the suite may
	// export KC_TEST_OPENSEARCH_URL, so pin index:none explicitly.
	body(t, kc(h, "store-set", "--index", "none"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy.body", ""), "--command-id", "schema-body", "--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	body(t, kc(h, "put", testObjectReference(t, core, "policy/A", ""), "--command-id", "i1", "--value", `{"body":"needs a runbook"}`))
	expectCode(t, kc(h, "search", "--repo", core, "--query", "runbook"), "CAPABILITY_UNSATISFIED")
}

func TestServeProjectionWorkerCatchesCommitWithoutSync(t *testing.T) {
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required")
	}
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

	body(t, governor("grant", "add", "--principal", provider,
		"--action", "writer.commit,writer.preview,knowledge.read",
		"--repo", repositoryID))
	drafts := writeProviderDrafts(t)
	published := asMap(t, body(t, asProvider("writer", "commit", "--command-id", "source-1", "--repo", repositoryID, "--dir", drafts)))
	commit := publishedCommit(t, published)

	body(t, governor("grant", "add", "--principal", consumer,
		"--action", "inventory.read,dataset.resolve,file.read"))
	body(t, governor("grant", "add", "--principal", consumer,
		"--action", "knowledge.read,knowledge.search,knowledge.schema.read",
		"--repo", repositoryID))
	body(t, governor("dataset", "define", "--dataset", setID, "--revision", "1",
		"--source", repositoryID))

	frozen := asMap(t, body(t, asConsumer("read", testObjectReference(t, repositoryID, "runbook/payment-oncall", ""), "--dataset", setID)))
	if frozen["reference"] != testObjectReference(t, repositoryID, "runbook/payment-oncall", "") || asMap(t, frozen)["commit"] != commit {
		t.Fatalf("published dataset %#v", frozen)
	}
	search := waitRemoteSearchHits(t, func() kcRunResult {
		return asConsumer("search", "--dataset", setID, "--query", "冻结窗口")
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
