package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	indexcli "kc/declarativeindex/capability"
	serv "kc/server"
)

// The product argv crosses the owned Discovery CLI/SDK and the actual Server
// route. A Home with no authority, layout or provider cannot construct an Index;
// static explanation needs only the explicit principal's current permission.
func TestProductPluginHelpUsesActualServerWithoutRuntimeConstruction(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_DATASET", "unrelated-inherited-dataset")
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "allow.json"), []byte(`{"version":2,"rules":[{"id":"help-reader","principal":"agent:help-reader","actions":["inventory.read"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	actual := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{AuthMode: "local"})
	if closer, ok := actual.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/object/v1/plugins/declarative-index" || r.URL.RawQuery != "" || r.ContentLength != 0 || r.Header.Get("X-Kc-As") != "agent:help-reader" {
			t.Errorf("module SDK changed the authorized targetless request: method=%s path=%s query=%s length=%d principal=%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.ContentLength, r.Header.Get("X-Kc-As"))
		}
		actual.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	result := kcRemote(t, server.URL, "agent:help-reader", "plugin", "declarative-index", "help")
	payload := body(t, result)
	var output indexcli.PluginAffordance
	if result.Status != 0 || json.Unmarshal([]byte(result.Stdout), &output) != nil {
		t.Fatalf("actual Server explanation failed: %#v", result)
	}
	wantJSON, err := json.Marshal(indexcli.Affordance())
	var wantPayload any
	if err != nil || json.Unmarshal(wantJSON, &wantPayload) != nil {
		t.Fatal("registered explanation is not valid JSON", err)
	}
	if requests.Load() != 1 || !reflect.DeepEqual(payload, wantPayload) || !reflect.DeepEqual(output, indexcli.Affordance()) || output.Summary == "" || !strings.Contains(output.Usage, "text/filter/sort") || !strings.Contains(output.Usage, "kc schema describe") || !strings.Contains(output.Usage, "--eq amount=100") {
		t.Fatalf("CLI did not deliver the actual registered declaration/query explanation: requests=%d output=%#v", requests.Load(), output)
	}
	t.Run("current_permission_is_required", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(home, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		expectCode(t, kcRemote(t, server.URL, "agent:help-reader", "plugin", "declarative-index", "help"), "FORBIDDEN")
		if requests.Load() != 2 {
			t.Fatalf("permission refusal did not use exactly one actual Server request: %d", requests.Load())
		}
	})
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		// The application may record its normal audit evidence. Everything
		// else would mount, configure or acquire a runtime in this empty Home.
		if entry.IsDir() || (entry.Name() != "allow.json" && entry.Name() != "audit.jsonl") {
			t.Fatalf("explanation constructed runtime state: %s", entry.Name())
		}
	}
}
