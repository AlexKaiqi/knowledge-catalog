package cli

import (
	"encoding/json"
	inv "kc/invocation"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedRepositoryCreateUsesTypedClientWithoutInventoryDiscovery(t *testing.T) {
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/snapshot/v1/connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.Header.Get("X-Kc-As") != "agent:creator" {
			t.Errorf("missing caller identity: %#v", r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if want := map[string]any{"name": "source"}; !reflect.DeepEqual(body, want) {
			t.Errorf("create request = %#v, want %#v", body, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repositoryId":"kr://managed/source","status":"APPLIED","head":"initial"}`))
	}))
	defer server.Close()
	result := Run([]string{"create", "--name", "source", "--server", server.URL, "--as", "agent:creator"})
	if result.Status != 0 || !strings.Contains(result.Stdout, `"status": "APPLIED"`) || calls != 1 {
		t.Fatalf("create: calls=%d result=%#v", calls, result)
	}
}

func TestCreateURLUsesTypedConnectionWithoutAttaching(t *testing.T) {
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	credential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(credential, []byte("secret-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/snapshot/v1/connections:connect" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["repository"] != "source" || body["url"] != "https://lakefs.example/source" || body["credential"] != "secret-token" {
			t.Fatalf("connection request: %#v", body)
		}
		_, _ = w.Write([]byte(`{"repositoryId":"source","status":"APPLIED"}`))
	}))
	defer server.Close()
	result := Run([]string{"create", "--url", "https://lakefs.example/source", "--credential-file", credential, "--server", server.URL, "--as", "kaiqidong"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "source") {
		t.Fatalf("create --url: %#v", result)
	}
}

func TestManagedRepositoryCreateRequiresOneProductSourceAndRejectsHostFlags(t *testing.T) {
	t.Setenv("KC_SERVER_URL", "http://127.0.0.1:1")
	for name, args := range map[string][]string{
		"missing source":         {"create"},
		"mixed source":           {"create", "--name", "source", "--url", "https://lakefs.example/source", "--credential-file", "credential.json"},
		"name with credential":   {"create", "--name", "source", "--credential-file", "credential.json"},
		"url without credential": {"create", "--url", "https://lakefs.example/source"},
		"url with store":         {"create", "--url", "https://lakefs.example/source", "--credential-file", "credential.json", "--store", "lakefs"},
	} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("%s must fail before auth/connection: %#v", name, result)
		}
	}
	base := []string{"create", "--name", "source"}
	for _, name := range []string{"catalog", "repo", "command-id", "driver", "dir", "dsn", "principal", "action", "ref", "dataset", "on-behalf-of"} {
		result := Run(append(append([]string{}, base...), "--"+name, "forbidden"))
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") || !strings.Contains(result.Stdout, "--"+name) {
			t.Errorf("create accepted %s: %#v", name, result)
		}
	}
}

func TestManagedRepositoryCreateHelpAndWriteLock(t *testing.T) {
	result := Run([]string{"create", "--help"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "kc create") {
		t.Fatalf("create help unavailable: %#v", result)
	}
	if surface, ok := cliSurface["create"]; !ok || surface.Action != "repositories.create" || inv.TypedInvocationReadOnly(surface.Action) {
		t.Fatalf("create must have its own mutating action: %#v", surface)
	}
}

func TestManagedRepositoryCreateHTTPRejectsCallerProvisioningFields(t *testing.T) {
	handler := serv.HTTPHandlerWithOptions(t.TempDir(), serv.HTTPServerOptions{})
	defer handler.(interface{ Close() error }).Close()
	for _, name := range []string{"driver", "dir", "dsn", "principal", "actions", "creator"} {
		request := httptest.NewRequest(http.MethodPost, "/snapshot/v1/repositories:create", strings.NewReader(`{"repository":"repo-A","commandId":"create-one","`+name+`":"forbidden"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:creator")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "USAGE_INVALID") {
			t.Errorf("HTTP accepted %s: %d %s", name, response.Code, response.Body.String())
		}
	}
}
