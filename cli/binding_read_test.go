package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func postAny(t *testing.T, base, path string, body map[string]any) any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-As", "agent:http-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status %d body %#v", path, resp.StatusCode, payload)
	}
	return payload
}

func TestWorkspaceReadHydratesStateBindingThroughTypedKnowledgeAPI(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "put", "--command-id", "binding-read", "--repo", repositoryID,
		"--object", "Service:orders", "--aspect", "health", "--value", "null",
		"--value-source", `{"kind":"binding","binding":{"mode":"state","runtime":"health","protocol":"health/v1","operations":{"read":{"call":"health.read"}}}}`))
	body(t, kc(home, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", repositoryID+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read", "--repo", repositoryID))
	pin := asMap(t, body(t, kc(home, "resolve", "--workspace", "agent")))
	commit := asMap(t, pin["repositories"])[repositoryID].(string)

	// The standalone binary has no wall-out runtime. It must fail rather than
	// returning the declaration's null placeholder as if it were knowledge.
	expectCode(t, kc(home, "read", "--workspace", "agent", "--object", "Service:orders"), "CAPABILITY_UNSATISFIED")

	var runtimeCalls atomic.Int32
	stateRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtimeCalls.Add(1)
		if r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Request-Id") == "" {
			t.Fatalf("State runtime request: %s %#v", r.URL.Path, r.Header)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["runtime"] != "health" || request["operation"] != "read" || request["call"] != "health.read" {
			t.Fatalf("State runtime payload: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{"status": "healthy"},
			"basis": map[string]any{
				"bindingGeneration": "health-runtime-v2", "consistency": "bounded",
				"sourceRevision": "health-88", "observedAt": "2026-08-27T09:00:00Z",
			},
		})
	}))
	defer stateRuntime.Close()
	lookup, err := cli.NewHTTPStateLookup(stateRuntime.URL, stateRuntime.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	read := postAny(t, server.URL, "/knowledge/v1/objects:read", map[string]any{
		"workspace": "agent", "object": "Service:orders", "aspect": "health",
	}).([]any)
	if len(read) != 1 {
		t.Fatalf("%#v", read)
	}
	result := asMap(t, read[0])
	if asMap(t, result["value"])["status"] != "healthy" || result["commit"] != commit {
		t.Fatalf("hydrated read: commit got=%#v want=%#v value=%#v", result["commit"], commit, result["value"])
	}
	observations := result["observations"].([]any)
	if len(observations) != 1 {
		t.Fatalf("observation envelope: %#v", result)
	}
	observation := asMap(t, observations[0])
	if observation["declarationCommit"] != commit || observation["declarationDigest"] == "" || asMap(t, observation["basis"])["sourceRevision"] != "health-88" {
		t.Fatalf("observation basis: %#v", observation)
	}
	if runtimeCalls.Load() != 1 {
		t.Fatalf("expected one State lookup, got %d", runtimeCalls.Load())
	}

	log := asMap(t, body(t, kc(home, "access-log", "--action", "knowledge.read")))
	entries := log["entries"].([]any)
	found := false
	for _, rawEntry := range entries {
		entry := asMap(t, rawEntry)
		if entry["result"] != "RESOLVED" {
			continue
		}
		accessed := entry["knowledge"].([]any)
		if len(accessed) == 1 && len(asMap(t, accessed[0])["observations"].([]any)) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("access evidence did not retain the observation basis: %#v", entries)
	}
}
