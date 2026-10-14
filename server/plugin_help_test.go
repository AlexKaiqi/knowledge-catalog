package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginHelpIsAuthorizedAndDoesNotMountOrExecuteSources(t *testing.T) {
	dir := t.TempDir()
	// There is deliberately no Home layout, authority, plugin work ledger or
	// search backend here. Explanation must succeed without constructing them.
	if err := os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[{"id":"help-reader","principal":"reader","actions":["inventory.read"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	handler := HTTPHandlerWithOptions(dir, HTTPServerOptions{AuthMode: "local"})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for _, test := range []struct {
		name, principal, suffix string
		status                  int
	}{
		{"registered", "reader", "declarative-index", 200},
		{"unknown", "reader", "unknown", 400},
		{"unauthenticated", "", "declarative-index", 401},
		{"forbidden", "other", "declarative-index", 403},
		{"unexpected-target", "reader", "declarative-index?repo=private", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, server.URL+"/object/v1/plugins/"+test.suffix, nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.principal != "" {
				request.Header.Set("X-Kc-As", test.principal)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("status=%d expected=%d body=%s", response.StatusCode, test.status, body)
			}
			if test.status == 200 {
				var result map[string]any
				if json.Unmarshal(body, &result) != nil || result["name"] != "declarative-index" || result["usage"] == "" {
					t.Fatalf("missing registered explanation: %s", body)
				}
				for _, runtime := range []string{"ready", "instances", "configuration", "credentials"} {
					if _, exists := result[runtime]; exists {
						t.Fatalf("explanation invented runtime fact %s", runtime)
					}
				}
			}
		})
	}
}
