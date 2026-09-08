package knowledge_test

import (
	"encoding/json"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestSchemaAccessPreservesLogicalScalarsAndUnindexedCompositeValues(t *testing.T) {
	for _, kind := range []string{"", "string", "boolean", "number", "integer", "date", "datetime", "timestamp", "object_ref", "object_ref_list"} {
		t.Run("access/"+kind, func(t *testing.T) {
			_, err := knowledge.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{
					"value": map[string]any{"type": kind, "access": []any{"text", "filter", "sort"}},
				},
			})
			if err != nil {
				t.Fatalf("defined scalar access must remain valid: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		kind  string
		value any
	}{
		{"object", map[string]any{"nested": "value"}},
		{"record", map[string]any{"nested": "value"}},
		{"array", []any{"text", true, map[string]any{"nested": "value"}}},
		{"relation_endpoint_list", []any{map[string]any{"objectId": "sample/A"}}},
	} {
		t.Run("unindexed/"+tc.kind, func(t *testing.T) {
			definition, err := knowledge.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": tc.kind}},
			})
			if err != nil {
				t.Fatal(err)
			}
			address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "sample/A"}
			if err := knowledge.ValidateSchemaInstance(address, map[string]any{"value": tc.value}, definition); err != nil {
				t.Fatalf("unindexed composite Canonical values must remain valid: %v", err)
			}
		})
	}
}

func TestSchemaInstanceValidationPreservesJSONNumbers(t *testing.T) {
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "sample/A"}
	for _, tc := range []struct {
		kind  string
		value json.Number
		valid bool
	}{
		{"integer", "9007199254740993", true},
		{"integer", "1.0", true},
		{"integer", "1e3", true},
		{"integer", "9007199254740993.1", false},
		{"integer", "1e-3", false},
		{"integer", "1e-999999999", false},
		{"integer", "0e-999999999", true},
		{"number", "9007199254740993.1", true},
		{"number", "1.25e-3", true},
		{"number", "NaN", false},
		{"number", "1e9999", false},
		{"number", "01", false},
	} {
		t.Run(tc.kind+"/"+string(tc.value), func(t *testing.T) {
			definition, err := knowledge.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": tc.kind}},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = knowledge.ValidateSchemaInstance(address, map[string]any{"value": tc.value}, definition)
			if tc.valid && err != nil {
				t.Fatalf("valid exact JSON number rejected: %v", err)
			}
			if !tc.valid && kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid {
				t.Fatalf("invalid JSON number accepted: %v", err)
			}
		})
	}
}

func TestSchemaAccessPreservesReferenceListsAndMemberScalars(t *testing.T) {
	definition, err := knowledge.ParseSchemaDefinition("schema/sample/columns/v1", map[string]any{
		"entity": "Sample", "aspect": "columns", "pattern": "keyed_collection",
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "access": []any{"text", "filter"}},
			"refs": map[string]any{"type": "object_ref_list", "access": []any{"filter", "sort"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"first", "second"} {
		address := knowledge.Address{Kind: knowledge.KindMember, ObjectID: "sample/A", AspectName: "columns", MemberKey: member}
		if err := knowledge.ValidateSchemaInstance(address, map[string]any{
			"name": member, "refs": []any{"sample/B", "sample/C"},
		}, definition); err != nil {
			t.Fatalf("defined multivalue scalar access must remain valid: %v", err)
		}
	}
}
