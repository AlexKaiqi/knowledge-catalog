package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedRepositoryCreateUsesTypedClientWithoutCatalogDiscovery(t *testing.T) {
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	const catalogID = "kr://managed/catalog"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/catalog/v1/repositories" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.Header.Get("X-Kc-As") != "agent:creator" {
			t.Errorf("missing caller identity: %#v", r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if want := map[string]any{"name": "source", "catalog": catalogID}; !reflect.DeepEqual(body, want) {
			t.Errorf("create request = %#v, want %#v", body, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"catalog":"kr://managed/catalog","repositoryId":"kr://managed/source","status":"APPLIED","head":"initial"}`))
	}))
	defer server.Close()
	if err := persistClientCatalog(server.URL, catalogID); err != nil {
		t.Fatal(err)
	}
	result := Run([]string{"create", "--name", "source", "--server", server.URL, "--as", "agent:creator"})
	if result.Status != 0 || !strings.Contains(result.Stdout, `"status": "APPLIED"`) || calls != 1 {
		t.Fatalf("create: calls=%d result=%#v", calls, result)
	}
}

func TestCreateURLUsesTypedConnectionWithoutAttaching(t *testing.T) {
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	const catalogID = "kr://managed/catalog"
	credential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(credential, []byte("secret-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/catalog/v1/catalogs/kr:%2F%2Fmanaged%2Fcatalog/repositories:connect" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["repository"] != "kr://git.example/acme/source" || body["url"] != "https://git.example/acme/source.git" || body["credential"] != "secret-token" {
			t.Fatalf("connection request: %#v", body)
		}
		_, _ = w.Write([]byte(`{"repositoryId":"kr://git.example/acme/source","status":"APPLIED"}`))
	}))
	defer server.Close()
	if err := persistClientCatalog(server.URL, catalogID); err != nil {
		t.Fatal(err)
	}
	result := Run([]string{"create", "--url", "https://git.example/acme/source.git", "--credential-file", credential, "--server", server.URL, "--as", "kaiqidong"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "kr://git.example/acme/source") {
		t.Fatalf("create --url: %#v", result)
	}
}

func TestManagedRepositoryCreateRequiresOneProductSourceAndRejectsHostFlags(t *testing.T) {
	t.Setenv("KC_SERVER_URL", "http://127.0.0.1:1")
	for name, args := range map[string][]string{
		"missing source":         {"create"},
		"mixed source":           {"create", "--name", "source", "--url", "https://git.example/acme/source", "--credential-file", "credential.json"},
		"name with credential":   {"create", "--name", "source", "--credential-file", "credential.json"},
		"url without credential": {"create", "--url", "https://git.example/acme/source"},
		"url with store":         {"create", "--url", "https://git.example/acme/source", "--credential-file", "credential.json", "--store", "gitea"},
	} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("%s must fail before auth/connection: %#v", name, result)
		}
	}
	base := []string{"create", "--name", "source"}
	for _, name := range []string{"catalog", "repo", "command-id", "driver", "dir", "dsn", "principal", "action", "ref", "workspace", "on-behalf-of"} {
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
	if surface, ok := cliSurface["create"]; !ok || surface.Action != "catalog.repositories.create" || typedInvocationReadOnly(surface.Action) {
		t.Fatalf("create must have its own mutating action: %#v", surface)
	}
}

func TestManagedRepositoryCreateHTTPRejectsCallerProvisioningFields(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{})
	defer handler.(interface{ Close() error }).Close()
	for _, name := range []string{"driver", "dir", "dsn", "principal", "actions", "creator"} {
		request := httptest.NewRequest(http.MethodPost, "/catalog/v1/catalogs/catalog-A/repositories:create", strings.NewReader(`{"repository":"repo-A","commandId":"create-one","`+name+`":"forbidden"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:creator")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "USAGE_INVALID") {
			t.Errorf("HTTP accepted %s: %d %s", name, response.Code, response.Body.String())
		}
	}
}
