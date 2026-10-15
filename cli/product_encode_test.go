package cli

import (
	declarativeindexcli "kc/declarativeindex/cli"
	discoverycli "kc/discovery/cli"
	inv "kc/invocation"
	accesscli "kc/liveaccess/cli"
	objectcli "kc/object/cli"
	"reflect"
	"testing"
)

func TestShapeCLIProductDelegatesModuleViews(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		path  string
		value map[string]any
		shape func(string, any) any
	}{
		{"read", map[string]any{"repository": "r", "commit": "c1", "reference": "urn:kc:object:note/hello", "address": map[string]any{"objectId": "note/hello"}, "digest": "unit-digest", "value": "body"}, objectcli.Shape},
		{"relations", map[string]any{"searchView": "internal", "hits": []any{}}, discoverycli.Shape},
		{"operations projection describe", map[string]any{"repository": "r", "status": "READY"}, declarativeindexcli.Shape},
		{"access", map[string]any{"observations": []any{map[string]any{"value": "observed"}}}, accesscli.Shape},
	} {
		t.Run(test.path, func(t *testing.T) {
			want := test.shape(test.path, test.value)
			if got := shapeCLIProduct(test.path, nil, test.value); !reflect.DeepEqual(got, want) {
				t.Fatalf("host did not select the owning module's product view: got %#v, want %#v", got, want)
			}
		})
	}
}

func TestEncodeCLIProductPreservesTransportResults(t *testing.T) {
	t.Parallel()
	for _, result := range []RunResult{
		{Status: 1, Stdout: `{"error":{"code":"FORBIDDEN"}}`},
		{Status: 0, Stdout: "help is text\n"},
		{Status: 0, Stdout: ""},
		{Status: 0, Stdout: "{invalid JSON"},
	} {
		if got := encodeCLIProduct("read", nil, result); !reflect.DeepEqual(got, result) {
			t.Fatalf("product encoding rewrote a transport result: got %#v, want %#v", got, result)
		}
	}
	for _, status := range []int{0, 2} {
		result := encodeCLIProduct("access", nil, RunResult{Status: status, Stdout: `{"observations":[{"value":"observed"}]}`})
		if result.Status != status || result.Stdout != inv.JSONOut(map[string]any{"value": "observed"}) {
			t.Fatalf("product encoding lost the status or module view: %#v", result)
		}
	}
}

func TestShapeCLIProductHostCommands(t *testing.T) {
	t.Parallel()
	hitmap := shapeCLIProduct("operations audit hitmap", nil, map[string]any{
		"source": "access",
		"hits":   []any{map[string]any{"objectId": "metric/gmv", "hits": 2}},
	})
	gotHitmap, _ := hitmap.(map[string]any)
	if gotHitmap["source"] != "hitmap" {
		t.Fatalf("hitmap source: %#v", hitmap)
	}
	if _, ok := gotHitmap["hits"]; !ok {
		t.Fatalf("hitmap dropped hits: %#v", hitmap)
	}

	// Module-owned results are passed through; its SDK/CLI contract tests own
	// grant inventory privacy and filtering, rather than a second host shaper.
	moduleResult := map[string]any{
		"version":       2,
		"initialGrants": map[string]any{"kr://x": "digest"},
		"rules": []any{
			map[string]any{"id": "alw_1", "principal": "bot", "repo": "kr://x", "actions": []any{"object.read"}},
			map[string]any{"id": "alw_2", "principal": "bot", "actions": []any{"inventory.read"}},
		},
	}
	if listed := shapeCLIProduct("grant list", map[string]FlagValue{"repo": "kr://x"}, moduleResult); !reflect.DeepEqual(listed, moduleResult) {
		t.Fatalf("host reinterpreted the module result: %#v", listed)
	}
}
