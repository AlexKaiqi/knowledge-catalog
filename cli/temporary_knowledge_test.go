package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

// Product Run -> typed HTTP -> application, with fixture creation confined to
// the test harness. The pin carries an unpublished definition across commands.
func TestProductTemporaryPinConsumesFrozenKnowledgeAndCurrentPermissions(t *testing.T) {
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
	body(t, govern("grant", "add", "--principal", consumer, "--action", "catalog.read,workspace.resolve,workspace.consume", "--catalog", cat))
	grant := asMap(t, body(t, govern("grant", "add", "--principal", consumer, "--action", "knowledge.read,knowledge.provenance,knowledge.history.read,knowledge.search", "--repo", repo)))
	body(t, govern("writer", "put", "--command-id", "temp-schema", "--repo", repo, "--object", "schema/note", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	first := publishedCommit(t, asMap(t, body(t, govern("writer", "put", "--command-id", "temp-first", "--repo", repo, "--object", "note/one", "--schema-ref", "schema/note", "--value", `{"body":"first"}`))))
	searchAvailable := os.Getenv("KC_TEST_OPENSEARCH_URL") != ""
	if searchAvailable {
		body(t, govern("operations", "projection", "sync", "--repo", repo))
	}
	before := asMap(t, body(t, govern("show")))
	pinPath := filepath.Join(t.TempDir(), "task-pin.json")
	body(t, invoke(consumer, "catalog", "use", cat))
	body(t, invoke(consumer, "workspace", "pin", "--source", repo, "--out", pinPath))
	raw, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["definition"] == nil || saved["catalog"] != cat {
		t.Errorf("temporary task pin must carry its definition: %s", raw)
	}
	body(t, govern("writer", "put", "--command-id", "temp-second", "--repo", repo, "--object", "note/one", "--schema-ref", "schema/note", "--value", `{"body":"second"}`))
	if searchAvailable {
		body(t, govern("operations", "projection", "sync", "--repo", repo))
		for _, args := range [][]string{
			{"knowledge", "search", "--pin", pinPath, "--query", "first"},
			{"knowledge", "search", "--repo", repo, "--commit", first, "--query", "first"},
		} {
			result := asMap(t, body(t, invoke(consumer, args...)))
			if len(result["hits"].([]any)) != 1 || searchHitObjectID(t, result) != "note/one" {
				t.Fatalf("SEARCH did not reuse first commit: %#v", result)
			}
		}
		latest := asMap(t, body(t, invoke(consumer, "knowledge", "search", "--repo", repo, "--ref", "refs/heads/main", "--query", "second")))
		if len(latest["hits"].([]any)) != 1 || searchHitObjectID(t, latest) != "note/one" {
			t.Fatalf("explicit latest ref SEARCH: %#v", latest)
		}
	} else {
		expectCode(t, invoke(consumer, "knowledge", "search", "--repo", repo, "--commit", first, "--query", "first"), "CAPABILITY_UNSATISFIED")
	}
	for _, operation := range []string{"read", "resolve", "provenance"} {
		values := body(t, invoke(consumer, "knowledge", operation, "--pin", pinPath, "--object", "note/one")).([]any)
		if len(values) != 1 || asMap(t, values[0])["commit"] != first {
			t.Fatalf("%s drifted from task pin: %#v", operation, values)
		}
	}
	read := body(t, invoke(consumer, "knowledge", "read", "--pin", pinPath, "--object", "note/one")).([]any)
	if asMap(t, asMap(t, read[0])["value"])["body"] != "first" {
		t.Fatalf("pin read latest: %#v", read)
	}
	after := asMap(t, body(t, govern("show")))
	a, _ := json.Marshal(before["workspaces"])
	b, _ := json.Marshal(after["workspaces"])
	if string(a) != string(b) {
		t.Fatalf("temporary consume published a Workspace: %s -> %s", a, b)
	}
	body(t, govern("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, invoke(consumer, "knowledge", "read", "--pin", pinPath, "--object", "note/one"), "FORBIDDEN")
}
