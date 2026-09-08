package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagedProductCreateNeedsOnlyName(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/catalog/v1/repositories" {
			t.Errorf("unexpected discovery or route: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 1 || body["name"] != "数据说明" {
			t.Errorf("caller had to supply infrastructure: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"repositoryId": "kr://kaiqidong/one", "name": "数据说明", "owner": "kaiqidong", "managementURL": "https://kc.example/repositories/one", "status": "APPLIED"})
	}))
	defer srv.Close()
	r := Run([]string{"catalog", "repo", "create", "--name", "数据说明", "--server", srv.URL, "--as", "kaiqidong"})
	if r.Status != 0 || calls != 1 || !strings.Contains(r.Stdout, "managementURL") {
		t.Fatalf("simple create blocked: calls=%d %#v", calls, r)
	}
}

func TestManagedProductMyRepositoriesAvoidsCatalogDiscovery(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/catalog/v1/repositories" {
			t.Errorf("requires broad Catalog discovery: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"repositories":[{"repositoryId":"kr://kaiqidong/one","managementURL":"https://kc.example/repositories/one"}]}`))
	}))
	defer srv.Close()
	r := Run([]string{"catalog", "repo", "list", "--mine", "--server", srv.URL, "--as", "kaiqidong"})
	if r.Status != 0 || !strings.Contains(r.Stdout, "managementURL") {
		t.Fatalf("cannot find own management URL: %#v", r)
	}
}
