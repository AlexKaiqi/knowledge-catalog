package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

// Product Run -> typed HTTP -> application. Exact historical replay is
// --repo --commit from a prior hit; consumers do not manage pin files.
func TestProductRepoCommitFreezesKnowledgeAndCurrentPermissions(t *testing.T) {
	home := testkit.TempDir(t)
	cat := "kr://acme/temporary"
	repo := "kr://acme/temporary-source"
	admin, consumer := "agent:admin", "agent:consumer"
	body(t, kc(home, "local", "init", "--catalog", cat))
	seedRepo(t, home, repo)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	invoke := func(principal string, args ...string) kcRunResult {
		isolateClientCredentials(t)
		all := append([]string{"--server", server.URL, "--as", principal}, args...)
		return kcRunResultFrom(cli.Run(all), publicCommandPath(all))
	}
	govern := func(args ...string) kcRunResult { return invoke(admin, args...) }
	body(t, govern("grant", "add", "--principal", consumer, "--action", "catalog.read,dataset.resolve,file.read", "--catalog", cat))
	grant := asMap(t, body(t, govern("grant", "add", "--principal", consumer, "--action", "knowledge.read,knowledge.provenance,knowledge.history.read,knowledge.search", "--repo", repo)))
	body(t, govern("writer", "put", "--command-id", "temp-schema", "--repo", repo, "--object", "schema/note", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	first := publishedCommit(t, asMap(t, body(t, govern("writer", "put", "--command-id", "temp-first", "--repo", repo, "--object", "note/one", "--schema-ref", "schema/note", "--value", `{"body":"first"}`))))
	searchAvailable := os.Getenv("KC_TEST_OPENSEARCH_URL") != ""
	if searchAvailable {
		body(t, govern("operations", "projection", "sync", "--repo", repo))
	}
	before := asMap(t, body(t, govern("show")))
	body(t, invoke(consumer, "catalog", "use", cat))
	body(t, govern("writer", "put", "--command-id", "temp-second", "--repo", repo, "--object", "note/one", "--schema-ref", "schema/note", "--value", `{"body":"second"}`))
	if searchAvailable {
		body(t, govern("operations", "projection", "sync", "--repo", repo))
		for _, args := range [][]string{
			{"search", "--repo", repo, "--commit", first, "--query", "first"},
		} {
			result := asMap(t, body(t, invoke(consumer, args...)))
			if len(result["hits"].([]any)) != 1 || searchHitObjectID(t, result) != "note/one" {
				t.Fatalf("SEARCH did not reuse first commit: %#v", result)
			}
		}
		latest := asMap(t, body(t, invoke(consumer, "search", "--repo", repo, "--ref", "refs/heads/main", "--query", "second")))
		if len(latest["hits"].([]any)) != 1 || searchHitObjectID(t, latest) != "note/one" {
			t.Fatalf("explicit latest ref SEARCH: %#v", latest)
		}
	} else {
		expectCode(t, invoke(consumer, "search", "--repo", repo, "--commit", first, "--query", "first"), "CAPABILITY_UNSATISFIED")
	}
	firstHit := func(t *testing.T, value any) map[string]any {
		t.Helper()
		if items, ok := value.([]any); ok {
			if len(items) != 1 {
				t.Fatalf("want one object, got %#v", value)
			}
			return asMap(t, items[0])
		}
		return asMap(t, value)
	}
	for _, operation := range []string{"read", "resolve", "provenance"} {
		got := firstHit(t, body(t, invoke(consumer, operation, "--repo", repo, "--commit", first, "--object", "note/one")))
		if got["commit"] != first {
			t.Fatalf("%s drifted from --repo --commit: %#v", operation, got)
		}
	}
	read := firstHit(t, body(t, invoke(consumer, "read", "--repo", repo, "--commit", first, "--object", "note/one")))
	if asMap(t, read["value"])["body"] != "first" {
		t.Fatalf("historical read latest: %#v", read)
	}
	after := asMap(t, body(t, govern("show")))
	a, _ := json.Marshal(before["datasets"])
	b, _ := json.Marshal(after["datasets"])
	if string(a) != string(b) {
		t.Fatalf("repo --commit consume published a Dataset: %s -> %s", a, b)
	}
	body(t, govern("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, invoke(consumer, "read", "--repo", repo, "--commit", first, "--object", "note/one"), "FORBIDDEN")
	expectCode(t, invoke(consumer, "read", "--repo", repo, "--object", "note/one"), "FORBIDDEN")
}
