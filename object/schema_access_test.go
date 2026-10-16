package object_test

import (
	"encoding/json"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

// Every logical scalar stays a valid structure type; retrieval over it is an
// index declaration, so the same field with access is rejected (S-01/K-26).
func TestSchemaPreservesLogicalScalarsAndUnindexedCompositeValues(t *testing.T) {
	for _, kind := range []string{"", "string", "boolean", "number", "integer", "date", "datetime", "timestamp", "object_ref", "object_ref_list"} {
		t.Run("scalar/"+kind, func(t *testing.T) {
			_, err := objectmodel.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": kind}},
			})
			if err != nil {
				t.Fatalf("defined scalar structure must remain valid: %v", err)
			}
			_, err = objectmodel.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{
					"value": map[string]any{"type": kind, "access": []any{"text", "filter", "sort"}},
				},
			})
			if kernel.CodeOf(err) != kernel.ErrSchemaUnsupported {
				t.Fatalf("scalar field access must be rejected in favour of an index declaration: %v", err)
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
			definition, err := objectmodel.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": tc.kind}},
			})
			if err != nil {
				t.Fatal(err)
			}
			address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "sample/A"}
			if err := objectmodel.ValidateSchemaInstance(address, map[string]any{"value": tc.value}, definition); err != nil {
				t.Fatalf("unindexed composite Canonical values must remain valid: %v", err)
			}
		})
	}
}

func TestSchemaInstanceValidationPreservesJSONNumbers(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "sample/A"}
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
			definition, err := objectmodel.ParseSchemaDefinition("schema/sample/v1", map[string]any{
				"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": tc.kind}},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = objectmodel.ValidateSchemaInstance(address, map[string]any{"value": tc.value}, definition)
			if tc.valid && err != nil {
				t.Fatalf("valid exact JSON number rejected: %v", err)
			}
			if !tc.valid && kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid {
				t.Fatalf("invalid JSON number accepted: %v", err)
			}
		})
	}
}

func TestSchemaPreservesReferenceListsAndMemberScalars(t *testing.T) {
	definition, err := objectmodel.ParseSchemaDefinition("schema/sample/columns/v1", map[string]any{
		"entity": "Sample", "aspect": "columns", "pattern": "keyed_collection",
		"fields": map[string]any{
			"name": map[string]any{"type": "string"},
			"refs": map[string]any{"type": "object_ref_list"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"first", "second"} {
		address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "sample/A", AspectName: "columns", MemberKey: member}
		if err := objectmodel.ValidateSchemaInstance(address, map[string]any{
			"name": member, "refs": []any{"sample/B", "sample/C"},
		}, definition); err != nil {
			t.Fatalf("defined multivalue scalar structure must remain valid: %v", err)
		}
	}
}
