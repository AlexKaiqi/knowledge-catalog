package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteGroupedCLIUsesTypedKnowledgeClient(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/knowledge/v1/objects:read" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"objectId": request["object"]}})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	t.Setenv("KC_HOME", t.TempDir())
	result := Run([]string{"knowledge", "read", "--workspace", "agent", "--object", "policy/A"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["workspace"] != "agent" || request["object"] != "policy/A" {
		t.Fatalf("typed request %#v", request)
	}
}

func TestRemoteCLIRejectsHomeAndMissingPrincipal(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	t.Setenv("KC_AS", "")
	result := Run([]string{"--server", server.URL, "--home", t.TempDir(), "identity", "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted --home")
	}
	result = Run([]string{"--server", server.URL, "identity", "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted an implicit owner")
	}
}

func TestLocalGroupNeverRoutesThroughServerDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("kc local command reached HTTP")
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	home := t.TempDir()
	result := Run([]string{"--home", home, "local", "init", "--catalog", "kr://local/catalog"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	result = Run([]string{"--server", server.URL, "local", "status"})
	if result.Status == 0 {
		t.Fatal("kc local accepted --server")
	}
}
