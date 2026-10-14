package cli

import (
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
}

func TestSearchDailyProductView(t *testing.T) {
	t.Parallel()
	search := Shape("search", map[string]any{
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
}
