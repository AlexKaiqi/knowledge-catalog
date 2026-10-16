package cli

import (
	"reflect"
	"testing"
)

func TestRelationsDailyProductView(t *testing.T) {
	t.Parallel()
	relations := Shape("relations", map[string]any{
		"retrievalEvidenceId": "rt_rel",
		"searchView":          map[string]any{"snapshots": map[string]any{"kr://x": "c1"}},
		"hits": []any{
			map[string]any{
				"repository":   "kr://x",
				"commit":       "c1",
				"objectId":     "team/finance",
				"objectRef":    map[string]any{"object": "team/finance"},
				"matchedRoles": []any{"owner"},
				"relation":     map[string]any{"relationType": "owned-by", "relationId": "relation/owned"},
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
	if relHit["objectId"] != "team/finance" || relHit["relationType"] != "owned-by" {
		t.Fatalf("relations neighbor: %#v", relHit)
	}
	if _, ok := relHit["relation"]; ok {
		t.Fatalf("relations kept relation envelope: %#v", relHit)
	}
}

func TestSearchDailyProductView(t *testing.T) {
	t.Parallel()
	search := Shape("search", map[string]any{
		"completeness":        "complete",
		"retrievalEvidenceId": "rt_1",
		"searchView":          map[string]any{"snapshots": map[string]any{"kr://x": "c1"}},
		"hits": []any{
			map[string]any{
				"object": map[string]any{
					"objectRef":  map[string]any{"object": "note/hello"},
					"repository": "kr://x",
					"commit":     "c1",
					"value":      map[string]any{"body": "secret"},
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
	if _, ok := hit["object"]; ok {
		t.Fatalf("search hit kept nested object: %#v", hit)
	}
	if _, ok := hit["value"]; ok {
		t.Fatalf("search hit kept body: %#v", hit)
	}
}

func TestSearchProductViewPreservesDeclarationAndSourceVersionEvidence(t *testing.T) {
	evidence := []any{map[string]any{"provider": "declarative-index", "lane": "lexical", "members": []any{map[string]any{"repository": "r-index", "commit": "c-index", "member": map[string]any{"objectId": "index/task", "aspectName": "index", "memberKey": "name"}, "role": "text"}}}}
	sources := []any{map[string]any{"repository": "r-one", "commit": "c-one"}, map[string]any{"repository": "r-two", "commit": "c-two"}}
	indexObservations := []any{map[string]any{"address": "urn:kc:object:service/orders#/health", "valueBasis": "snapshot:c-old", "observationId": "obs-old"}}
	version := map[string]any{"objectId": "service/orders", "sources": sources, "units": []any{map[string]any{"valueBasis": "snapshot:c-one"}}}
	result := Shape("search", map[string]any{"hits": []any{map[string]any{"object": map[string]any{"repository": "r-one", "commit": "c-one", "objectId": "service/orders", "reference": "urn:kc:object:service/orders", "sources": sources, "value": map[string]any{"body": "secret"}}, "version": version, "evidence": evidence, "indexObservations": indexObservations}}}).(map[string]any)
	hit := result["hits"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(hit["indexObservations"], indexObservations) || !reflect.DeepEqual(hit["evidence"], evidence) || !reflect.DeepEqual(hit["sources"], sources) || !reflect.DeepEqual(hit["version"], version) || hit["reference"] != "urn:kc:object:service/orders" {
		t.Fatalf("fixed source/member evidence was lost: %#v", hit)
	}
	if hit["value"] != nil || hit["object"] != nil {
		t.Fatalf("CLI search view included Canonical body: %#v", hit)
	}
}
