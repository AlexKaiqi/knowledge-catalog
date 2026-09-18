package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
)

type taihuProofAuthenticator struct{}

func (taihuProofAuthenticator) Name() string { return "taihu" }

func (taihuProofAuthenticator) Authenticate(_ context.Context, headers http.Header) (cli.HTTPIdentity, error) {
	if headers.Get("Authorization") == "" || headers.Get("X-Tai-Identity") == "" {
		return cli.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "missing Taihu caller proof")
	}
	return cli.HTTPIdentity{Principal: "alice", Provider: "taihu", Subject: "12345"}, nil
}

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
	var runtimeCalls atomic.Int32
	stateRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtimeCalls.Add(1)
		if r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Request-Id") == "" {
			t.Fatalf("State runtime request: %s %#v", r.URL.Path, r.Header)
		}
		if r.Header.Get("X-Resource-Principal") != "agent:http-test" {
			t.Fatalf("local principal: %#v", r.Header)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tai-Identity") != "" {
			t.Fatalf("local pairing must not forward unverified caller proof: %#v", r.Header)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["operation"] != "lookup" || request["call"] != "lookup" {
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
	body(t, kc(home, "put", "--command-id", "binding-schema", "--repo", repositoryID,
		"--object", "schema/service.health",
		"--value", `{"entity":"Service","aspect":"health","origin":"`+stateRuntime.URL+`","fields":{"status":{"type":"string","access":["filter"]}}}`))
	body(t, kc(home, "put", "--command-id", "binding-entity", "--repo", repositoryID,
		"--object", "Service:orders", "--aspect", "properties", "--value", `{"name":"orders"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repositoryID+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read", "--repo", repositoryID))
	commit := asMap(t, body(t, kc(home, "read", "--dataset", "agent", "--object", "Service:orders", "--aspect", "properties")).([]any)[0])["commit"].(string)

	lookup := cli.NewHTTPStateLookup(stateRuntime.Client())
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	read := postAny(t, server.URL, "/knowledge/v1/objects:read", map[string]any{
		"dataset": "agent", "object": "Service:orders", "aspect": "health",
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

	log := asMap(t, body(t, kc(home, "access-log")))
	entries, _ := log["entries"].([]any)
	found := false
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok || entry["result"] != "RESOLVED" {
			continue
		}
		accessed, _ := entry["knowledge"].([]any)
		if len(accessed) != 1 {
			continue
		}
		item, _ := accessed[0].(map[string]any)
		obs, _ := item["observations"].([]any)
		if len(obs) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("access evidence did not retain the observation basis: %#v", log)
	}
}

func TestTypedKnowledgeReadForwardsTaihuCallerAuthentication(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, home, repositoryID)
	stateRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer taihu-token" || r.Header.Get("X-Tai-Identity") != "signed.identity" {
			t.Fatalf("Taihu caller proof: %#v", r.Header)
		}
		if r.Header.Get("X-Resource-Principal") != "alice" {
			t.Fatalf("verified principal: %#v", r.Header)
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
	body(t, kc(home, "put", "--command-id", "binding-schema", "--repo", repositoryID,
		"--object", "schema/service.health",
		"--value", `{"entity":"Service","aspect":"health","origin":"`+stateRuntime.URL+`","fields":{"status":{"type":"string","access":["filter"]}}}`))
	body(t, kc(home, "put", "--command-id", "binding-entity", "--repo", repositoryID,
		"--object", "Service:orders", "--aspect", "properties", "--value", `{"name":"orders"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repositoryID+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "alice", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "alice", "--cmd", "read", "--repo", repositoryID))

	lookup := cli.NewHTTPStateLookup(stateRuntime.Client())
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{
		Authenticator: taihuProofAuthenticator{}, StateLookup: lookup,
	})
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	raw, err := json.Marshal(map[string]any{"dataset": "agent", "object": "Service:orders", "aspect": "health"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/objects:read", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer taihu-token")
	req.Header.Set("X-Tai-Identity", "signed.identity")
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
		t.Fatalf("Taihu hydrated read: status %d body %#v", resp.StatusCode, payload)
	}
	read := payload.([]any)
	if len(read) != 1 || asMap(t, asMap(t, read[0])["value"])["status"] != "healthy" {
		t.Fatalf("Taihu hydrated read: %#v", payload)
	}
}
