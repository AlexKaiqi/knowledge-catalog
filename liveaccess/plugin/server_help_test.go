package plugin_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"kc/cli"
	inv "kc/invocation"
	"kc/liveaccess"
	accesscli "kc/liveaccess/cli"
	"kc/server"
)

// U19: actual product argv and public Server deliver the module-owned static
// affordance even when no Home runtime can be opened.
func TestServerLiveAccessHelpUsesProductCLIWithoutHomeRuntime(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_DATASET", "unrelated")
	directory := t.TempDir()
	if err := inv.WriteAllow(directory, inv.AllowFile{Rules: []inv.AllowRule{{ID: "help", Principal: "help-reader", Actions: []string{"inventory.read"}}}}); err != nil {
		t.Fatal(err)
	}
	runtime := newAcceptanceRuntime(t, 11)
	handler := server.HTTPHandlerWithOptions(directory, server.HTTPServerOptions{AuthMode: "local", StateLookup: liveaccess.NewHTTPStateLookup(runtime.server.Client())})
	t.Cleanup(func() { _ = handler.(interface{ Close() error }).Close() })
	service := httptest.NewServer(handler)
	t.Cleanup(service.Close)
	call := func(name string) cli.RunResult {
		return cli.Run([]string{"--server", service.URL, "--as", "help-reader", "plugin", name, "help"})
	}
	result := call("liveaccess")
	var explanation accesscli.PluginAffordance
	if result.Status != 0 || json.Unmarshal([]byte(result.Stdout), &explanation) != nil || !reflect.DeepEqual(explanation, accesscli.Affordance()) {
		t.Fatalf("public CLI/Server lost owned explanation: %#v", result)
	}
	if unknown := call("unregistered-liveaccess"); unknown.Status == 0 || !strings.Contains(unknown.Stdout, "CAPABILITY_UNSATISFIED") {
		t.Fatalf("unknown explanation accepted: %#v", unknown)
	}
	if runtime.calls.Load() != 0 {
		t.Fatal("plugin help performed source I/O")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || (entry.Name() != "allow.json" && entry.Name() != "audit.jsonl") {
			t.Fatalf("static help constructed runtime state: %s", entry.Name())
		}
	}
}
