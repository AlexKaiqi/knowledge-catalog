package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductPluginHelpUsesRegisteredNameAndAuthenticatedTransport(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_DATASET", "unrelated-default")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/knowledge/v1/plugins/declarative-index" || r.Header.Get("X-Kc-As") != "agent:reader" || r.ContentLength != 0 || r.URL.RawQuery != "" {
			t.Errorf("plugin explanation did not use sole authenticated targetless task: %#v", r)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"declarative-index","summary":"fixed-basis queries","usage":"kc search","tasks":["search"]}`))
	}))
	t.Cleanup(server.Close)
	result := Run([]string{"--server", server.URL, "--as", "agent:reader", "plugin", "declarative-index", "help"})
	var output map[string]any
	if result.Status != 0 || json.Unmarshal([]byte(result.Stdout), &output) != nil || output["name"] != "declarative-index" || requests != 1 {
		t.Fatalf("description was not read once through Server: requests=%d result=%#v", requests, result)
	}
	for _, tail := range [][]string{{"plugin", "declarative-index", "install"}, {"plugin", "declarative-index", "help", "--repo", "secret"}, {"plugin", "declarative-index", "help", "extra"}} {
		invalid := Run(append([]string{"--server", server.URL, "--as", "agent:reader"}, tail...))
		if invalid.Status == 0 || !strings.Contains(invalid.Stdout, "USAGE_INVALID") {
			t.Fatalf("invalid dynamic plugin operands accepted: %v %#v", tail, invalid)
		}
	}
	if requests != 1 {
		t.Fatalf("rejected plugin task performed transport: %d calls", requests)
	}
}
