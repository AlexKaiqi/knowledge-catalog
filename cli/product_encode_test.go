package cli

import (
	"reflect"
	"testing"
)

func TestShapeCLIProductDailyCommands(t *testing.T) {
	t.Parallel()
	read := shapeCLIProduct("read", nil, map[string]any{
		"knowledgeRef": map[string]any{"object": "note/hello", "repository": "kr://x"},
		"repository":   "kr://x",
		"commit":       "c1",
		"address":      map[string]any{"objectId": "note/hello", "aspectName": "readme", "kind": "Aspect"},
		"value":        map[string]any{"body": "hi"},
		"units":        []any{map[string]any{"objectId": "note/hello"}},
		"declarations": []any{map[string]any{"schemaRef": "schema/core/readme/v1"}},
	})
	wantRead := map[string]any{
		"repository": "kr://x",
		"objectId":   "note/hello",
		"commit":     "c1",
		"value":      map[string]any{"body": "hi"},
		"aspectName": "readme",
		"schemaRef":  "schema/core/readme/v1",
	}
	if !reflect.DeepEqual(read, wantRead) {
		t.Fatalf("read product: %#v", read)
	}

	describe := shapeCLIProduct("schema describe", nil, map[string]any{
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
					map[string]any{"path": "body", "type": "string", "access": []any{"text"}},
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

	resolved := shapeCLIProduct("resolve", nil, map[string]any{
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

	relations := shapeCLIProduct("relations", nil, map[string]any{
		"retrievalEvidenceId": "rt_rel",
		"searchView":          map[string]any{"snapshots": map[string]any{"kr://x": "c1"}},
		"hits": []any{
			map[string]any{
				"repository":   "kr://x",
				"commit":       "c1",
				"objectId":     "Team:finance",
				"knowledgeRef": map[string]any{"object": "Team:finance"},
				"matchedRoles": []any{"owner"},
				"relation":     map[string]any{"relationType": "owned-by", "relationId": "relation:owned"},
				"evidence":     []any{map[string]any{"lane": "graph"}},
			},
		},
	})
	gotRel, _ := relations.(map[string]any)
	if _, ok := gotRel["searchView"]; ok {
		t.Fatalf("relations kept search envelope: %#v", relations)
	}
	if gotRel["retrievalEvidenceId"] != "rt_rel" {
		t.Fatalf("relations dropped evidence: %#v", relations)
	}
	relHits, _ := gotRel["hits"].([]any)
	if len(relHits) != 1 {
		t.Fatalf("relations hits: %#v", relations)
	}
	relHit, _ := relHits[0].(map[string]any)
	if relHit["objectId"] != "Team:finance" || relHit["relationType"] != "owned-by" {
		t.Fatalf("relations neighbor: %#v", relHit)
	}
	if _, ok := relHit["relation"]; ok {
		t.Fatalf("relations kept relation envelope: %#v", relHit)
	}

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

	search := shapeCLIProduct("search", nil, map[string]any{
		"completeness":        "complete",
		"retrievalEvidenceId": "rt_1",
		"searchView":          map[string]any{"snapshots": map[string]any{"kr://x": "c1"}},
		"hits": []any{
			map[string]any{
				"knowledge": map[string]any{
					"knowledgeRef": map[string]any{"object": "note/hello"},
					"repository":   "kr://x",
					"commit":       "c1",
					"value":        map[string]any{"body": "secret"},
				},
			},
		},
	})
	gotSearch, _ := search.(map[string]any)
	if gotSearch["retrievalEvidenceId"] != "rt_1" || gotSearch["completeness"] != "complete" {
		t.Fatalf("search kept protocol evidence: %#v", search)
	}
	hits, _ := gotSearch["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("search hits: %#v", search)
	}
	hit, _ := hits[0].(map[string]any)
	if hit["objectId"] != "note/hello" || hit["repository"] != "kr://x" || hit["commit"] != "c1" {
		t.Fatalf("search hit identity: %#v", hit)
	}
	if _, ok := hit["knowledge"]; ok {
		t.Fatalf("search hit kept nested knowledge: %#v", hit)
	}
	if _, ok := hit["value"]; ok {
		t.Fatalf("search hit kept body: %#v", hit)
	}

	access := shapeCLIProduct("access", nil, map[string]any{
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

	listed := shapeCLIProduct("grant list", map[string]FlagValue{"repo": "kr://x"}, map[string]any{
		"version":       2,
		"initialGrants": map[string]any{"kr://x": "digest"},
		"rules": []any{
			map[string]any{"id": "alw_1", "principal": "bot", "repo": "kr://x", "actions": []any{"knowledge.read"}},
			map[string]any{"id": "alw_2", "principal": "bot", "catalog": "kr://cat", "actions": []any{"catalog.read"}},
		},
	})
	gotList, _ := listed.(map[string]any)
	if _, ok := gotList["version"]; ok {
		t.Fatalf("grant list kept allow file header: %#v", listed)
	}
	rules, _ := gotList["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("grant list --repo filter: %#v", listed)
	}
	rule, _ := rules[0].(map[string]any)
	if rule["id"] != "alw_1" {
		t.Fatalf("grant list kept the wrong rule: %#v", listed)
	}
}
