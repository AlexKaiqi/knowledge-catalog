package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/snapshot/gitea"
)

func postVerb(t *testing.T, base, verb string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+"/v1/"+verb, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, payload
}

func mustPostVerb(t *testing.T, base, verb string, body map[string]any) map[string]any {
	t.Helper()
	status, payload := postVerb(t, base, verb, body)
	if status != http.StatusOK {
		t.Fatalf("POST /v1/%s: status %d body %#v", verb, status, payload)
	}
	return payload
}

// TestVFSOverHTTP drives vfs-write/vfs-read/vfs-list through the real kc
// serve HTTP facade — the transport an external agent harness plugin
// actually calls — mixing a real local git member with a real Gitea member
// (docker; skips if unavailable). This is the same acceptance shape as
// catalog.TestLoomAcceptanceMixedGiteaAndLocal, one layer up: over HTTP,
// through the CLI's verb table, not the Go API directly.
func TestVFSOverHTTP(t *testing.T) {
	giteaBase, token, run := testkit.GiteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)

	home := testkit.TempDir(t)
	server := httptest.NewServer(cli.HTTPHandler(home))
	defer server.Close()

	alice := "kr://acme/personals/alice"
	sum := sha256.Sum256([]byte(alice + run))
	semantic := "kr://acme/public/semantic"
	dsn := giteaBase + "/kc/kc-" + hex.EncodeToString(sum[:8])

	mustPostVerb(t, server.URL, "init", map[string]any{"catalog": "kr://acme/catalog"})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": alice})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": semantic, "driver": "gitea", "dsn": dsn})
	mustPostVerb(t, server.URL, "define-workspace", map[string]any{
		"workspace": "notes", "revision": 1,
		"source": []string{alice + "=refs/heads/main@", semantic + "=refs/heads/main@refs/semantic"},
	})

	content := base64.StdEncoding.EncodeToString([]byte("draft over http\n"))
	written := mustPostVerb(t, server.URL, "vfs-write", map[string]any{
		"workspace": "notes", "command-id": "http-1", "path": "analysis/churn.md", "content": content,
	})
	if asMap(t, written["result"])["newCommit"] == "" {
		t.Fatalf("vfs-write must advance a commit: %#v", written)
	}

	metric := base64.StdEncoding.EncodeToString([]byte("gitea over http\n"))
	mustPostVerb(t, server.URL, "vfs-write", map[string]any{
		"workspace": "notes", "command-id": "http-2", "path": "refs/semantic/metrics/dau.md", "content": metric,
	})

	read := mustPostVerb(t, server.URL, "vfs-read", map[string]any{"workspace": "notes", "path": "refs/semantic/metrics/dau.md"})
	if read["repository"] != semantic {
		t.Fatalf("must route to the gitea member: %#v", read)
	}
	got, err := base64.StdEncoding.DecodeString(read["content"].(string))
	if err != nil || string(got) != "gitea over http\n" {
		t.Fatalf("%q %v", got, err)
	}

	listing := mustPostVerb(t, server.URL, "vfs-list", map[string]any{"workspace": "notes"})
	entries := listing["entries"].([]any)
	paths := map[string]bool{}
	for _, raw := range entries {
		paths[asMap(t, raw)["path"].(string)] = true
	}
	if !paths["analysis/churn.md"] || !paths["refs/semantic/metrics/dau.md"] {
		t.Fatalf("listing must cover both engines over HTTP: %v", paths)
	}
}

func TestVFSHTTPReplaysInlineWorkspacePin(t *testing.T) {
	home := testkit.TempDir(t)
	server := httptest.NewServer(cli.HTTPHandler(home))
	defer server.Close()

	repo := "kr://acme/code/app"
	mustPostVerb(t, server.URL, "init", map[string]any{"catalog": "kr://acme/catalog"})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": repo})
	mustPostVerb(t, server.URL, "define-workspace", map[string]any{
		"workspace": "agent", "revision": 1, "source": []string{repo + "=refs/heads/main@"},
	})
	v1 := base64.StdEncoding.EncodeToString([]byte("version one\n"))
	first := mustPostVerb(t, server.URL, "vfs-write", map[string]any{
		"workspace": "agent", "command-id": "pin-v1", "path": "state.txt", "content": v1,
	})
	firstCommit := asMap(t, first["result"])["newCommit"]
	pin := mustPostVerb(t, server.URL, "resolve", map[string]any{"workspace": "agent"})
	if asMap(t, pin["repositories"])[repo] != firstCommit {
		t.Fatalf("pin must capture V1: %#v", pin)
	}

	v2 := base64.StdEncoding.EncodeToString([]byte("version two\n"))
	mustPostVerb(t, server.URL, "vfs-write", map[string]any{
		"workspace": "agent", "command-id": "pin-v2", "path": "state.txt", "content": v2,
	})
	live := mustPostVerb(t, server.URL, "vfs-read", map[string]any{"workspace": "agent", "path": "state.txt"})
	frozen := mustPostVerb(t, server.URL, "vfs-read", map[string]any{
		"workspace": "agent", "path": "state.txt", "pin": pin,
	})
	decode := func(payload map[string]any) string {
		raw, err := base64.StdEncoding.DecodeString(payload["content"].(string))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if decode(live) != "version two\n" || decode(frozen) != "version one\n" {
		t.Fatalf("live=%q frozen=%q", decode(live), decode(frozen))
	}
	listed := mustPostVerb(t, server.URL, "vfs-list", map[string]any{"workspace": "agent", "pin": pin})
	entries := listed["entries"].([]any)
	var stateCommit any
	for _, entry := range entries {
		row := asMap(t, entry)
		if row["path"] == "state.txt" {
			stateCommit = row["commit"]
		}
	}
	if stateCommit != firstCommit {
		t.Fatalf("pinned list must expose the same V1 coordinate: %#v", listed)
	}
}

func TestVFSWriteReplaysSameIntentAfterServiceReopen(t *testing.T) {
	home := testkit.TempDir(t)
	repo := "kr://acme/code/restart"
	server := httptest.NewServer(cli.HTTPHandler(home))
	mustPostVerb(t, server.URL, "init", map[string]any{"catalog": "kr://acme/catalog"})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": repo})
	mustPostVerb(t, server.URL, "define-workspace", map[string]any{
		"workspace": "agent", "revision": 1, "source": []string{repo + "=refs/heads/main@"},
	})
	payload := map[string]any{
		"workspace": "agent", "command-id": "restart-safe", "path": "state.txt",
		"content": base64.StdEncoding.EncodeToString([]byte("persisted\n")),
	}
	first := mustPostVerb(t, server.URL, "vfs-write", payload)
	firstCommit := asMap(t, first["result"])["newCommit"]
	server.Close()

	server = httptest.NewServer(cli.HTTPHandler(home))
	defer server.Close()
	replayed := mustPostVerb(t, server.URL, "vfs-write", payload)
	if replayed["disposition"] != "REPLAYED" || asMap(t, replayed["result"])["newCommit"] != firstCommit {
		t.Fatalf("restart replay = %#v, first commit = %v", replayed, firstCommit)
	}
	pin := mustPostVerb(t, server.URL, "resolve", map[string]any{"workspace": "agent"})
	if asMap(t, pin["repositories"])[repo] != firstCommit {
		t.Fatalf("replay moved HEAD: %#v", pin)
	}
}

func TestHTTPConcurrentIdenticalVFSWritesApplyOnce(t *testing.T) {
	home := testkit.TempDir(t)
	repo := "kr://acme/code/concurrent"
	server := httptest.NewServer(cli.HTTPHandler(home))
	defer server.Close()
	mustPostVerb(t, server.URL, "init", map[string]any{"catalog": "kr://acme/catalog"})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": repo})
	mustPostVerb(t, server.URL, "define-workspace", map[string]any{
		"workspace": "agent", "revision": 1, "source": []string{repo + "=refs/heads/main@"},
	})
	payload, err := json.Marshal(map[string]any{
		"workspace": "agent", "command-id": "one-logical-write", "path": "race.txt",
		"content": base64.StdEncoding.EncodeToString([]byte("once\n")),
	})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		body   map[string]any
		err    error
	}
	results := make(chan result, 24)
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post(server.URL+"/v1/vfs-write", "application/json", bytes.NewReader(payload))
			if err != nil {
				results <- result{err: err}
				return
			}
			defer resp.Body.Close()
			var body map[string]any
			err = json.NewDecoder(resp.Body).Decode(&body)
			results <- result{status: resp.StatusCode, body: body, err: err}
		}()
	}
	wg.Wait()
	close(results)
	applied, replayed := 0, 0
	var commit any
	for got := range results {
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("concurrent write failed: status=%d body=%#v err=%v", got.status, got.body, got.err)
		}
		switch got.body["disposition"] {
		case "APPLIED":
			applied++
		case "REPLAYED":
			replayed++
		default:
			t.Fatalf("unexpected disposition: %#v", got.body)
		}
		gotCommit := asMap(t, got.body["result"])["newCommit"]
		if commit == nil {
			commit = gotCommit
		} else if commit != gotCommit {
			t.Fatalf("one command produced multiple commits: %v and %v", commit, gotCommit)
		}
	}
	if applied != 1 || replayed != 23 {
		t.Fatalf("applied=%d replayed=%d", applied, replayed)
	}
}
