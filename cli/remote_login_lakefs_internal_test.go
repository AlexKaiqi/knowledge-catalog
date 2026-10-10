package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLakeFSLoginDiscoveryUsesExistingIssuedTokenMode(t *testing.T) {
	isolateLoginConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/v1/auth":
			_, _ = io.WriteString(w, `{"mode":"lakefs","accepts":["Cookie","Authorization"],"localAssertion":false}`)
		case "/identity/v1/whoami":
			if r.Header.Get("Authorization") != "Bearer caller-token" || r.Header.Get("X-Kc-As") != "" {
				t.Error("CLI token was not paired with the caller proof")
			}
			_, _ = io.WriteString(w, `{"principal":"console-validation"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	result := Run([]string{"login", "--server", server.URL})
	if result.Status == 0 || !strings.Contains(result.Stdout, "--auth lakefs; use --mode token") || strings.Contains(result.Stdout, "unknown auth mode") {
		t.Fatalf("known native verifier advertised as unknown: %s", result.Stdout)
	}
	result = Run([]string{"login", "--server", server.URL, "--mode", "local", "--as", "platform-admin"})
	if result.Status == 0 {
		t.Fatal("native verifier accepted local identity assertion")
	}
	result = Run([]string{"login", "--server", server.URL, "--mode", "token", "--token", "caller-token"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "console-validation") {
		t.Fatalf("existing issued-token login failed: %s", result.Stdout)
	}
}
