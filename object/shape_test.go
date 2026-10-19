package object

import (
	"reflect"
	"testing"

	"kc/kernel"
)

func TestParseShapeCompilesAspectMemberAndFieldSelections(t *testing.T) {
	shape, err := ParseShape(`{
		definition
		interfaces(keys: ["create-order", "a/b"]) { path owner { team } }
		monitoring { alerts }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, selection := range shape.Selections() {
		got = append(got, selection.Locator())
	}
	want := []string{"/definition", "/interfaces/create-order", "/interfaces/a~1b", "/monitoring"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("locators = %v, want %v", got, want)
	}
	selections := shape.Selections()
	if selections[0].Fields != nil {
		t.Fatalf("bare aspect must select the complete value, got %#v", selections[0].Fields)
	}
	if !reflect.DeepEqual(selections[2].Path(), []string{"interfaces", "a/b"}) {
		t.Fatalf("result path keeps the decoded member key, got %v", selections[2].Path())
	}
	wantFields := []ShapeField{{Name: "path"}, {Name: "owner", Children: []ShapeField{{Name: "team"}}}}
	if !reflect.DeepEqual(selections[1].Fields, wantFields) {
		t.Fatalf("fields = %#v", selections[1].Fields)
	}
}

func TestParseShapeRejectsEverythingBeyondProjection(t *testing.T) {
	for name, text := range map[string]string{
		"syntax":           `{ definition`,
		"alias":            `{ d: definition }`,
		"fragment spread":  `{ ...F } fragment F on X { definition }`,
		"inline fragment":  `{ ... on X { definition } }`,
		"directive":        `{ definition @include(if: true) }`,
		"variable":         `query($k: [String!]) { interfaces(keys: $k) }`,
		"operation name":   `query Named { definition }`,
		"mutation":         `mutation { definition }`,
		"two operations":   `{ definition } { runbook }`,
		"duplicate aspect": `{ definition definition }`,
		"duplicate field":  `{ definition { name name } }`,
		"other argument":   `{ interfaces(first: 2) }`,
		"nested argument":  `{ interfaces { path(keys: ["x"]) } }`,
		"empty keys":       `{ interfaces(keys: []) }`,
		"empty key":        `{ interfaces(keys: [""]) }`,
		"repeated key":     `{ interfaces(keys: ["a", "a"]) }`,
		"non-string key":   `{ interfaces(keys: [1]) }`,
		"scalar keys":      `{ interfaces(keys: "a") }`,
		"empty":            `{ }`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseShape(text); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("ParseShape(%q) err = %v, want %s", text, err, kernel.ErrUsageInvalid)
			}
		})
	}
}

func TestProjectShapeFieldsSeparatesAbsentFromLegitimateNull(t *testing.T) {
	value := map[string]any{"path": "/orders", "owner": nil, "extra": "dropped", "nested": map[string]any{"a": 1, "b": 2}}
	fields := []ShapeField{{Name: "path"}, {Name: "owner"}, {Name: "missing"}, {Name: "nested", Children: []ShapeField{{Name: "a"}}}}
	got, errs := ProjectShapeFields(value, fields, []string{"interfaces", "create"})
	want := map[string]any{"path": "/orders", "owner": nil, "missing": nil, "nested": map[string]any{"a": 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected = %#v", got)
	}
	if len(errs) != 1 || !reflect.DeepEqual(errs[0].Path, []string{"interfaces", "create", "missing"}) || errs[0].Code != kernel.ErrObjectRefUnresolved {
		t.Fatalf("only the absent field has an error, got %#v", errs)
	}
	if whole, errs := ProjectShapeFields(value, nil, nil); !reflect.DeepEqual(whole, value) || errs != nil {
		t.Fatalf("nil fields keep the complete value")
	}
	if _, errs := ProjectShapeFields("scalar", fields, []string{"x"}); len(errs) != 1 || errs[0].Code != kernel.ErrUsageInvalid {
		t.Fatalf("fields on a scalar are reported, got %#v", errs)
	}
}

func TestShapeSchemaMismatchesCoverNamesAndLevels(t *testing.T) {
	fields := []ShapeField{{Name: "path"}, {Name: "ghost"}, {Name: "owner", Children: []ShapeField{{Name: "team"}}}, {Name: "spec", Children: []ShapeField{{Name: "x"}}}}
	schema := SchemaDefinition{Fields: []SchemaFieldDefinition{{Path: "path", Type: "string"}, {Path: "owner", Type: "string"}, {Path: "spec", Type: "object"}}}
	want := []string{"ghost is not declared", "owner is string and has no sub-fields"}
	if got := ShapeSchemaMismatches(fields, schema); !reflect.DeepEqual(got, want) {
		t.Fatalf("closed schema mismatches = %v", got)
	}
	schema.AdditionalProperties = true
	if got := ShapeSchemaMismatches(fields, schema); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("open schema still rejects wrong levels only, got %v", got)
	}
}
