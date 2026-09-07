package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestManagedRepositoryCreateUsesTypedClientWithoutCatalogDiscovery(t *testing.T) {
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	const catalogID = "kr://managed/catalog"
	const repositoryID = "kr://managed/source"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/catalog/v1/catalogs/"+url.PathEscape(catalogID)+"/repositories:create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.Header.Get("X-Kc-As") != "agent:creator" {
			t.Errorf("missing caller identity: %#v", r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if want := map[string]any{"repository": repositoryID, "commandId": "create-one"}; !reflect.DeepEqual(body, want) {
			t.Errorf("create request = %#v, want %#v", body, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"catalog":"kr://managed/catalog","repositoryId":"kr://managed/source","commandId":"create-one","status":"APPLIED","head":"initial"}`))
	}))
	defer server.Close()
	result := Run([]string{"catalog", "repo", "create", "--catalog", catalogID, "--repo", repositoryID, "--command-id", "create-one", "--server", server.URL, "--as", "agent:creator"})
	if result.Status != 0 || !strings.Contains(result.Stdout, `"status": "APPLIED"`) || calls != 1 {
		t.Fatalf("create: calls=%d result=%#v", calls, result)
	}
}

func TestManagedRepositoryCreateRequiresExplicitCoordinatesAndRejectsHostFlags(t *testing.T) {
	t.Setenv("KC_CATALOG", "kr://ambient/catalog")
	t.Setenv("KC_SERVER_URL", "http://127.0.0.1:1")
	base := []string{"catalog", "repo", "create", "--catalog", "kr://managed/catalog", "--repo", "kr://managed/source", "--command-id", "create-one"}
	for _, missing := range []string{"catalog", "repo", "command-id"} {
		args := []string{}
		for i := 0; i < len(base); i++ {
			if base[i] == "--"+missing {
				i++
				continue
			}
			args = append(args, base[i])
		}
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") || !strings.Contains(result.Stdout, "--"+missing) {
			t.Errorf("missing %s must fail before auth/connection: %#v", missing, result)
		}
	}
	for _, name := range []string{"driver", "dir", "dsn", "principal", "action", "ref", "workspace", "on-behalf-of"} {
		result := Run(append(append([]string{}, base...), "--"+name, "forbidden"))
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") || !strings.Contains(result.Stdout, "--"+name) {
			t.Errorf("create accepted %s: %#v", name, result)
		}
	}
}

func TestManagedRepositoryCreateHelpAndWriteLock(t *testing.T) {
	result := Run([]string{"catalog", "repo", "create", "--help"})
	for _, term := range []string{"--catalog", "--repo", "--command-id"} {
		if result.Status != 0 || !strings.Contains(result.Stdout, term) {
			t.Errorf("create help omits %s: %#v", term, result)
		}
	}
	if surface, ok := cliSurface["catalog repo create"]; !ok || surface.Action != "catalog.repositories.create" || typedInvocationReadOnly(surface.Action) {
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
