package cli

import (
	"reflect"
	"testing"
)

func TestReadDailyProductView(t *testing.T) {
	t.Parallel()
	read := Shape("read", map[string]any{
		"reference":    "urn:kc:object:note/hello#/readme",
		"locator":      "/readme",
		"objectRef":    map[string]any{"object": "note/hello", "repository": "kr://x"},
		"repository":   "kr://x",
		"commit":       "c1",
		"address":      map[string]any{"objectId": "note/hello", "aspectName": "readme", "kind": "Aspect"},
		"value":        map[string]any{"body": "hi"},
		"units":        []any{map[string]any{"objectId": "note/hello"}},
		"declarations": []any{map[string]any{"schemaRef": "schema/core/readme/v1"}},
	})
	wantRead := map[string]any{
		"reference":    "urn:kc:object:note/hello#/readme",
		"locator":      "/readme",
		"address":      map[string]any{"objectId": "note/hello", "aspectName": "readme", "kind": "Aspect"},
		"declarations": []any{map[string]any{"schemaRef": "schema/core/readme/v1"}},
		"repository":   "kr://x",
		"objectId":     "note/hello",
		"commit":       "c1",
		"value":        map[string]any{"body": "hi"},
		"aspectName":   "readme",
		"schemaRef":    "schema/core/readme/v1",
	}
	if !reflect.DeepEqual(read, wantRead) {
		t.Fatalf("read product: %#v", read)
	}
}

func TestSchemaDescribeDailyProductView(t *testing.T) {
	t.Parallel()
	describe := Shape("schema describe", map[string]any{
		"repository": "kr://x",
		"commit":     "c1",
		"schemas": []any{
			map[string]any{
				"objectId":             "schema/runbook.body",
				"entity":               "Runbook",
				"metaSchema":           "schema/meta/schema-definition/v1",
				"pattern":              "record",
				"additionalProperties": false,
				"digest":               "abc",
				"fields": []any{
					map[string]any{"path": "body", "type": "string"},
				},
			},
		},
	})
	gotDescribe, _ := describe.(map[string]any)
	if gotDescribe["repository"] != "kr://x" || gotDescribe["commit"] != "c1" {
		t.Fatalf("describe identity: %#v", describe)
	}
	schemas, _ := gotDescribe["schemas"].([]any)
	if len(schemas) != 1 {
		t.Fatalf("describe schemas: %#v", describe)
	}
	schema, _ := schemas[0].(map[string]any)
	if schema["objectId"] != "schema/runbook.body" {
		t.Fatalf("describe objectId: %#v", schema)
	}
	if _, ok := schema["entity"]; ok {
		t.Fatalf("describe kept list-like entity: %#v", schema)
	}
	fields, _ := schema["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("describe fields: %#v", schema)
	}
	field, _ := fields[0].(map[string]any)
	if field["path"] != "body" {
		t.Fatalf("describe field path: %#v", field)
	}
}

func TestResolveDailyProductView(t *testing.T) {
	t.Parallel()
	resolved := Shape("resolve", map[string]any{
		"repository": "kr://x",
		"commit":     "c1",
		"objectId":   "note/hello",
		"status":     "RESOLVED",
		"address":    map[string]any{"objectId": "note/hello", "aspectName": "readme", "memberKey": "user:bob"},
		"pathHint":   "notes/hello.yaml",
		"digest":     "d1",
	})
	gotResolved, _ := resolved.(map[string]any)
	if gotResolved["status"] != "RESOLVED" || gotResolved["objectId"] != "note/hello" || gotResolved["commit"] != "c1" {
		t.Fatalf("resolve identity: %#v", resolved)
	}
	if gotResolved["digest"] != "d1" {
		t.Fatalf("resolve must keep the update digest: %#v", resolved)
	}
	if gotResolved["aspectName"] != "readme" || gotResolved["memberKey"] != "user:bob" {
		t.Fatalf("resolve address fields: %#v", resolved)
	}
	if _, ok := gotResolved["address"]; ok {
		t.Fatalf("resolve kept address envelope: %#v", resolved)
	}
}
