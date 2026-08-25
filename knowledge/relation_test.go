package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestDecodeRelationRejectsNonCanonicalEnvelope(t *testing.T) {
	address := knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "rel-1"}
	validEndpoints := []any{
		map[string]any{"role": "input", "objectRef": "Table:a"},
		map[string]any{"role": "output", "objectRef": "Table:b"},
	}
	cases := map[string]map[string]any{
		"non-string id":       {"relationId": 1, "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints},
		"lowercase direction": {"relationId": "rel-1", "relationType": "lineage", "direction": "directed", "endpoints": validEndpoints},
		"non-string endpoint": {"relationId": "rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": []any{
			map[string]any{"role": "input", "objectRef": 1}, map[string]any{"role": "output", "objectRef": "Table:b"},
		}},
		"scalar attributes": {"relationId": "rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints, "attributes": "bad"},
		"scalar validity":   {"relationId": "rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints, "validity": "bad"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := knowledge.DecodeRelation(address, body)
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("error = %v, want %s", err, kernel.ErrUsageInvalid)
			}
		})
	}
}
