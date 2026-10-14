package cli

import (
	"testing"
)

func TestAccessDailyProductView(t *testing.T) {
	t.Parallel()
	access := Shape("access", map[string]any{
		"bindings": []any{map[string]any{
			"schemaRef": "schema/table.stats",
			"address":   map[string]any{"objectId": "table/orders", "aspectName": "stats"},
		}},
		"observations": []any{map[string]any{
			"value": map[string]any{"rowCount": 1},
			"basis": map[string]any{"consistency": "latest-only"},
		}},
	})
	gotAccess, _ := access.(map[string]any)
	if gotAccess["objectId"] != "table/orders" || gotAccess["aspectName"] != "stats" || gotAccess["schemaRef"] != "schema/table.stats" {
		t.Fatalf("access identity: %#v", access)
	}
	if asMap, _ := gotAccess["value"].(map[string]any); asMap["rowCount"] != 1 {
		t.Fatalf("access value: %#v", access)
	}
	if _, ok := gotAccess["bindings"]; ok {
		t.Fatalf("access kept protocol envelope: %#v", access)
	}
	if _, ok := gotAccess["observations"]; ok {
		t.Fatalf("access kept observations envelope: %#v", access)
	}
}
