package cli_test

import (
	"net/http/httptest"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestLocalDeploymentBootstrapsThenUsesServerClientBoundary(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/local-server"
	principal := "agent:local-admin"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	expectCode(t, kc(home, "local", "grant", "bootstrap"), "USAGE_INVALID")
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", principal))
	expectCode(t, kc(home, "local", "grant", "bootstrap", "--principal", "agent:other"), "PRECONDITION_FAILED")

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	result := cli.Run([]string{"--server", server.URL, "--as", principal,
		"catalog", "show", "--catalog", catalogID})
	if result.Status != 0 {
		t.Fatalf("server-backed local client failed: %s", result.Stdout)
	}
	state := asMap(t, body(t, kcRunResult{RunResult: result}))
	if state["catalogId"] != catalogID {
		t.Fatalf("server returned wrong catalog: %#v", state)
	}
}
