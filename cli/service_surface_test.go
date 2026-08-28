package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing(t *testing.T) {
	handler := cli.HTTPHandlerWithOptions(testkit.TempDir(t), cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	formal := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/identity/v1/whoami"},
		{http.MethodGet, "/catalog/v1/catalogs"},
		{http.MethodPost, "/knowledge/v1/objects:read"},
		{http.MethodPost, "/workspace-files/v1/tree:list"},
		{http.MethodPost, "/writer/v1/repositories/repo/commits"},
		{http.MethodPost, "/governance/v1/proposals"},
		{http.MethodGet, "/admin/v1/grants"},
		{http.MethodPost, "/operations/v1/projections:sync"},
	}
	for _, endpoint := range formal {
		request, err := http.NewRequest(endpoint.method, server.URL+endpoint.path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
			t.Fatalf("formal route is not registered: %s %s (%d)", endpoint.method, endpoint.path, response.StatusCode)
		}
	}

	retired := []string{
		"/v1/read", "/v1/init", "/knowledge/v1/list", "/vfs-read",
		"/workspace-files/v1/files:write", "/local/v1/init",
	}
	for _, path := range retired {
		request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("retired route %s returned %d, want 404", path, response.StatusCode)
		}
	}
}
