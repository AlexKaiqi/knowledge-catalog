package object_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestDecodeRelationRejectsNonCanonicalEnvelope(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "fixture/rel-1"}
	validEndpoints := []any{
		map[string]any{"role": "input", "object": "table/a"},
		map[string]any{"role": "output", "object": "table/b"},
	}
	envelope := func(endpoints []any) map[string]any {
		return map[string]any{"relationId": "fixture/rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": endpoints}
	}
	cases := map[string]map[string]any{
		"non-string id":       {"relationId": 1, "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints},
		"lowercase direction": {"relationId": "fixture/rel-1", "relationType": "lineage", "direction": "directed", "endpoints": validEndpoints},
		"non-string endpoint": envelope([]any{map[string]any{"role": "input", "object": 1}, validEndpoints[1]}),
		"repository-qualified endpoint": envelope([]any{
			map[string]any{"role": "input", "objectRef": map[string]any{"object": "table/a"}}, validEndpoints[1],
		}),
		"endpoint names its repository": envelope([]any{
			map[string]any{"role": "input", "object": "table/a", "repository": "kr://acme/core"}, validEndpoints[1],
		}),
		"endpoint id without namespace": envelope([]any{map[string]any{"role": "input", "object": "a"}, validEndpoints[1]}),
		"repeated endpoint":             envelope([]any{validEndpoints[0], validEndpoints[0]}),
		"scalar attributes":             {"relationId": "fixture/rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints, "attributes": "bad"},
		"scalar validity":               {"relationId": "fixture/rel-1", "relationType": "lineage", "direction": "DIRECTED", "endpoints": validEndpoints, "validity": "bad"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := objectmodel.DecodeRelation(address, body)
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("error = %v, want %s", err, kernel.ErrUsageInvalid)
			}
		})
	}
}

func TestDecodeRelationEndpointsAreObjectIDsOnly(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "fixture/rel-1"}
	relation, err := objectmodel.DecodeRelation(address, map[string]any{
		"relationId": "fixture/rel-1", "relationType": "lineage", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "input", "object": "table/a"},
			map[string]any{"role": "output", "object": "table/b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []objectmodel.RelationEndpoint{{Role: "input", Object: "table/a"}, {Role: "output", Object: "table/b"}}
	if len(relation.Endpoints) != len(want) || relation.Endpoints[0] != want[0] || relation.Endpoints[1] != want[1] {
		t.Fatalf("endpoints = %#v, want %#v", relation.Endpoints, want)
	}
}
