package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
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
		{http.MethodPost, "/knowledge/v1/resources:access"},
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

func TestAppendAndStreamSurfacesStayAbsent(t *testing.T) {
	home := testkit.TempDir(t)
	for _, command := range []string{"append", "stream"} {
		result := kc(home, command)
		if result.Status == 0 || !bytes.Contains([]byte(result.Stdout), []byte("unknown command")) {
			t.Fatalf("kc %s unexpectedly exists: %#v", command, result)
		}
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for _, path := range []string{
		"/writer/v1/repositories/repo/append",
		"/knowledge/v1/stream",
		"/operations/v1/streams:append",
	} {
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

func TestTypedServiceRequestsRejectUnknownFieldsAndTrailingJSON(t *testing.T) {
	handler := cli.HTTPHandlerWithOptions(testkit.TempDir(t), cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for name, body := range map[string]string{
		"unknown":  `{"workspace":"agent","object":"x","flags":"must-not-exist"}`,
		"trailing": `{"workspace":"agent","object":"x"} {}`,
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/objects:read", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var envelope map[string]any
		if response.StatusCode != http.StatusBadRequest || json.Unmarshal(raw, &envelope) != nil || asMap(t, envelope["error"])["code"] != "USAGE_INVALID" {
			t.Fatalf("%s status=%d body=%s", name, response.StatusCode, raw)
		}
	}
}

func TestXKcAsUsesTheSameAuthorizationRulesAsCLI(t *testing.T) {
	home := testkit.TempDir(t)
	repository := "kr://acme/public/http-auth"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "seed", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"one"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", "agent:http", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:http", "--cmd", "read", "--repo", repository))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for principal, want := range map[string]int{"agent:http": http.StatusOK, "agent:intruder": http.StatusForbidden} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/objects:read",
			bytes.NewBufferString(`{"workspace":"agent","object":"Policy:one"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", principal)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("principal=%s status=%d want=%d", principal, response.StatusCode, want)
		}
	}
}
