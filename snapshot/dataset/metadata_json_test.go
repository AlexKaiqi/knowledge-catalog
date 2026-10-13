package dataset_test

import (
	"encoding/json"
	"reflect"
	"testing"

	catalog "kc/snapshot/dataset"
)

func TestNestedDatasetMetadataJSONCannotBypassTypesOrVocabulary(t *testing.T) {
	for _, body := range []string{
		`{"metadata":null}`,
		`{"metadata":{"key":null}}`,
		`{"metadata":{"key":42}}`,
		`{"metadata":{"key":"first","key":"second"}}`,
		`{"metadata":{"key":"first"},"metadata":{"key":"second"}}`,
		`{"metadata":null,"metadata":{"key":"value"}}`,
		`{"metadata":{"key":"valid"},"unknownField":true}`,
		`{"sources":[{"repository":"source","unknownField":true}]}`,
	} {
		var wrapper struct {
			Definition *catalog.KnowledgeSet `json:"definition"`
		}
		if err := catalog.DecodeJSON([]byte(`{"definition":`+body+`}`), &wrapper); err == nil {
			t.Fatalf("nested Dataset bypassed its JSON contract: %s", body)
		}
		var recipe struct {
			Definition *catalog.KnowledgeSetRecipe `json:"definition"`
		}
		if err := catalog.DecodeJSON([]byte(`{"definition":`+body+`}`), &recipe); err == nil {
			t.Fatalf("nested recipe bypassed its JSON contract: %s", body)
		}
	}
	var wrapper struct {
		Definition catalog.KnowledgeSet `json:"definition"`
	}
	if err := catalog.DecodeJSON([]byte(`{"definition":{"setId":"delivery","revision":1,"metadata":{"custom":"value","empty":""}}}`), &wrapper); err != nil {
		t.Fatalf("valid nested metadata rejected: %v", err)
	}
	if !reflect.DeepEqual(wrapper.Definition.Metadata, map[string]string{"custom": "value", "empty": ""}) {
		t.Fatal("nested metadata changed", wrapper.Definition.Metadata)
	}
}

func TestDatasetMetadataJSONFollowsNestedTypesAndSkipsOpaqueBodies(t *testing.T) {
	type scope struct {
		Definition *catalog.KnowledgeSet `json:"definition"`
	}
	type request struct {
		scope
		Definitions []*catalog.KnowledgeSet          `json:"definitions"`
		ByName      map[string]*catalog.KnowledgeSet `json:"byName"`
		Metadata    any                              `json:"metadata"`
		Body        map[string]any                   `json:"body"`
		Raw         json.RawMessage                  `json:"raw"`
	}
	for _, body := range []string{
		`{"definition":{"metadata":{"key":null}}}`,
		`{"definitions":[{"metadata":{"key":42}}]}`,
		`{"byName":{"named":{"metadata":{"key":"first","key":"second"}}}}`,
	} {
		var target request
		if err := catalog.ValidateMetadataJSON([]byte(body), &target); err == nil {
			t.Fatalf("typed nested metadata escaped validation: %s", body)
		}
	}
	const body = `{"definition":{"metadata":{"key":"value"}},"definitions":[null],"byName":{"absent":null},"metadata":{"nested":[null,1]},"body":{"definition":{"metadata":{"key":null}}},"raw":{"definition":{"metadata":{"key":null}}}}`
	var target request
	if err := catalog.DecodeJSON([]byte(body), &target); err != nil {
		t.Fatalf("opaque business metadata was interpreted as Dataset metadata: %v", err)
	}
	if target.Definition.Metadata["key"] != "value" {
		t.Fatal("promoted Dataset definition was not decoded", target)
	}

	var shadowed struct {
		scope
		Definition any `json:"definition"`
	}
	if err := catalog.DecodeJSON([]byte(`{"definition":{"metadata":{"key":null}}}`), &shadowed); err != nil {
		t.Fatalf("shadowed opaque JSON field was interpreted as a Dataset: %v", err)
	}
}

func TestEmptyDatasetOverlayDoesNotShareMetadata(t *testing.T) {
	base := catalog.KnowledgeSet{Metadata: map[string]string{"custom": "base"}}
	merged, err := catalog.MergeOverlay(base, catalog.KnowledgeSetOverlay{})
	if err != nil || !reflect.DeepEqual(base, merged) {
		t.Fatalf("empty overlay changed definition: %#v %v", merged, err)
	}
	merged.Metadata["custom"] = "overlay result"
	if base.Metadata["custom"] != "base" {
		t.Fatal("empty overlay exposes the base metadata map")
	}
}
