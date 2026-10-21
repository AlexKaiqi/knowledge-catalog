package cli_test

import (
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
	serv "kc/server"
)

func TestDatasetHistoryRequiresSourceHistoryGrantOnEveryPage(t *testing.T) {
	home := testkit.TempDir(t)
	const repo = "kr://acme/history/source"
	const consumer = "agent:history-consumer"
	body(t, kc(home, "init"))
	seedRepo(t, home, repo)
	for _, revision := range []string{"first", "second"} {
		body(t, kc(home, "put", "urn:kc:object:note/history", "--repo", repo,
			"--command-id", revision, "--path-hint", "notes/history.yaml", "--value", `{"body":"`+revision+`"}`))
	}
	body(t, kc(home, "dataset", "define", "whole-history", "--revision", "1", "--source", repo+"@"))
	body(t, kc(home, "dataset", "define", "sliced-history", "--revision", "1", "--file-source", repo+"@notes/history.yaml@history.yaml"))
	body(t, kc(home, "grant", "add", "--principal", consumer, "--action", "file.read"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	consume := func(args ...string) kcRunResult { return kcRemote(t, server.URL, consumer, args...) }
	for _, dataset := range []string{"whole-history", "sliced-history"} {
		body(t, consume("read", "urn:kc:object:note/history", "--dataset", dataset))
		expectCode(t, consume("log", "--dataset", dataset, "--object", "note/history"), "FORBIDDEN")
	}
	// A source content grant must not substitute for its history action.
	readGrant := asMap(t, body(t, kc(home, "grant", "add", "--principal", consumer, "--action", "object.read", "--repo", repo)))
	expectCode(t, consume("log", "--dataset", "whole-history", "--object", "note/history"), "FORBIDDEN")
	body(t, kc(home, "grant", "remove", "--id", readGrant["id"].(string)))
	grant := asMap(t, body(t, kc(home, "grant", "add", "--principal", consumer, "--action", "object.history.read", "--repo", repo)))
	body(t, consume("log", "--repo", repo, "--object", "note/history"))
	expectCode(t, consume("read", "urn:kc:object:note/history", "--repo", repo), "FORBIDDEN")
	page := asMap(t, body(t, consume("log", "--dataset", "whole-history", "--object", "note/history", "--limit", "1")))
	cursor, ok := page["continuation"].(string)
	if !ok || cursor == "" {
		t.Fatalf("expected source history continuation: %#v", page)
	}
	next := asMap(t, body(t, consume("log", "--dataset", "whole-history", "--object", "note/history", "--limit", "1", "--continuation", cursor)))
	if len(next["logs"].([]any)) != 1 {
		t.Fatalf("authorized source history missing: %#v", next)
	}
	body(t, kc(home, "grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, consume("log", "--dataset", "whole-history", "--object", "note/history", "--limit", "1", "--continuation", cursor), "FORBIDDEN")
	body(t, consume("read", "urn:kc:object:note/history", "--dataset", "whole-history"))
}
