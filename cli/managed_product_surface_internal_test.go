package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kcclient "kc/client"
)

func TestManagedProductCreateNeedsOnlyName(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/catalog/v1/catalogs" {
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []any{map[string]any{"id": "kr://kaiqidong/catalog"}}})
			return
		}
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/catalog/v1/repositories" {
			t.Errorf("unexpected discovery or route: %s %s", r.Method, r.URL.Path)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "数据说明" {
			t.Errorf("caller had to supply infrastructure: %#v", body)
		}
		for _, forbidden := range []string{"repo", "commandId", "driver", "dsn", "dir"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("caller supplied forbidden field %s: %#v", forbidden, body)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"repositoryId": "kr://kaiqidong/one", "name": "数据说明", "owner": "kaiqidong", "managementURL": "https://kc.example/repositories/one", "status": "APPLIED"})
	}))
	defer srv.Close()
	r := Run([]string{"create", "--name", "数据说明", "--server", srv.URL, "--as", "kaiqidong"})
	if r.Status != 0 || calls != 1 || !strings.Contains(r.Stdout, "managementURL") {
		t.Fatalf("simple create blocked: calls=%d %#v", calls, r)
	}
}

func TestManagedProductMyRepositoriesAvoidsCatalogDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/catalog/v1/repositories" {
			t.Errorf("requires broad Catalog discovery: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"repositories":[{"repositoryId":"kr://kaiqidong/one","managementURL":"https://kc.example/repositories/one"}]}`))
	}))
	defer srv.Close()
	client, err := kcclient.New(kcclient.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := client.CatalogService().MyRepositories(context.Background(), kcclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "managementURL") {
		t.Fatalf("cannot find own management URL: %#v", out)
	}
}
