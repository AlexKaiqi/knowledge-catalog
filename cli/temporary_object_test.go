package cli_test

import (
	"encoding/json"
	serv "kc/server"
	"net/http/httptest"
	"os"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

// Product Run -> typed HTTP -> application. Exact historical replay is
// the source object's URN and prior commit; consumers do not manage pin files.
func TestProductRepoCommitFreezesObjectAndCurrentPermissions(t *testing.T) {
	home := testkit.TempDir(t)

	repo := "kr://acme/temporary-source"
	admin, consumer := "agent:admin", "agent:consumer"
	body(t, kc(home, "local", "init"))
	seedRepo(t, home, repo)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
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
	body(t, govern("grant", "add", "--principal", consumer, "--action", "inventory.read,dataset.resolve,file.read"))
	grant := asMap(t, body(t, govern("grant", "add", "--principal", consumer, "--action", "object.read,object.provenance,object.history.read,object.search", "--repo", repo)))
	body(t, govern("writer", "put", testObjectReference(t, repo, "schema/note", ""), "--repo", repo, "--command-id", "temp-schema", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	first := publishedCommit(t, asMap(t, body(t, govern("writer", "put", testObjectReference(t, repo, "note/one", ""), "--repo", repo, "--command-id", "temp-first", "--schema-ref", "schema/note", "--value", `{"body":"first"}`))))
	searchAvailable := os.Getenv("KC_TEST_OPENSEARCH_URL") != ""
	if searchAvailable {
		body(t, govern("operations", "projection", "sync", "--repo", repo))
	}
	before := asMap(t, body(t, govern("show")))
	body(t, govern("writer", "put", testObjectReference(t, repo, "note/one", ""), "--repo", repo, "--command-id", "temp-second", "--schema-ref", "schema/note", "--value", `{"body":"second"}`))
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
	for _, operation := range []string{"resolve", "provenance"} {
		got := asMap(t, body(t, invoke(consumer, operation, "--repo", repo, "--commit", first, "--object", "note/one")))
		if got["repository"] != repo || got["commit"] != first {
			t.Fatalf("%s drifted from --repo --commit: %#v", operation, got)
		}
	}
	reference := testObjectReference(t, repo, "note/one", "")
	read := asMap(t, body(t, invoke(consumer, "read", reference, "--repo", repo, "--commit", first)))
	if read["reference"] != reference || read["repository"] != repo || read["commit"] != first ||
		asMap(t, read["address"])["objectId"] != "note/one" || asMap(t, read["value"])["body"] != "first" {
		t.Fatalf("historical read latest: %#v", read)
	}
	after := asMap(t, body(t, govern("show")))
	a, _ := json.Marshal(before["datasets"])
	b, _ := json.Marshal(after["datasets"])
	if string(a) != string(b) {
		t.Fatalf("repo --commit consume published a Dataset: %s -> %s", a, b)
	}
	body(t, govern("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, invoke(consumer, "read", testObjectReference(t, repo, "note/one", ""), "--repo", repo, "--commit", first), "FORBIDDEN")
	expectCode(t, invoke(consumer, "read", testObjectReference(t, repo, "note/one", ""), "--repo", repo), "FORBIDDEN")
}
