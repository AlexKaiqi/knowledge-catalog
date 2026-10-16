package reader_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func constrainedDescriptionSchema() map[string]any {
	return map[string]any{
		"entity": "Order", "aspect": "definition", "pattern": "record", "description": "Order definition",
		"additionalProperties": false,
		"fields": map[string]any{
			"name":   map[string]any{"type": "string", "required": true},
			"target": map[string]any{"type": "object_ref", "required": true, "refTypes": []any{"Table", "Image"}},
		},
	}
}

func reportedSchemaDefinition(t *testing.T, description reader.SchemaDescription) objectmodel.SchemaDefinition {
	t.Helper()
	raw, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	definition, exists := wire["definition"]
	if !exists {
		t.Fatal("Schema description omits its complete normalized definition")
	}
	var result objectmodel.SchemaDefinition
	if err := json.Unmarshal(definition, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDescribeSchemaKeepsCompleteConstraintsAtFixedVersions(t *testing.T) {
	s := testkit.NewSetup(t, "kr://reader/schema-constraints")
	const id objectmodel.ObjectID = "schema/order.definition"
	first, err := s.Writer.Commit("schema-first", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: testkit.PutEntity(string(id), constrainedDescriptionSchema(), ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	changed := constrainedDescriptionSchema()
	changed["fields"].(map[string]any)["target"].(map[string]any)["required"] = false
	changed["fields"].(map[string]any)["target"].(map[string]any)["refTypes"] = []any{"Table", "Image", "Module"}
	second, err := s.Writer.Commit("schema-loosened", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.NewCommit, ExpectedTargetCommit: first.Result.NewCommit,
		Operations: testkit.PutEntity(string(id), changed, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := objectmodel.SchemaDefinition{
		ObjectID: id, MetaSchema: objectmodel.MetaSchemaV1, Entity: "Order", Aspect: "definition", Pattern: "record",
		Description: "Order definition", AdditionalProperties: false,
		Fields: []objectmodel.SchemaFieldDefinition{
			{Path: "name", Type: "string", Required: true},
			{Path: "target", Type: "object_ref", Required: true, RefTypes: []string{"image", "table"}},
		},
	}
	var previous reader.SchemaDescription
	for i, basis := range []kernel.CommitID{first.Result.NewCommit, second.Result.NewCommit} {
		report, err := s.Reader.DescribeSchema(s.RepositoryID, basis, id)
		if err != nil || report.Commit != basis || len(report.Schemas) != 1 {
			t.Fatalf("Schema report lost its fixed basis: %#v error=%v", report, err)
		}
		if i == 1 {
			want.Fields[1].Required = false
			want.Fields[1].RefTypes = []string{"image", "module", "table"}
		}
		actual := reportedSchemaDefinition(t, report.Schemas[0])
		if !reflect.DeepEqual(actual, want) || report.Schemas[0].Digest != kernel.CanonicalDigest(want) {
			t.Fatalf("fixed Schema constraints/digest: got=%#v digest=%s want=%#v", actual, report.Schemas[0].Digest, want)
		}
		if i == 1 && (report.Schemas[0].Digest == previous.Digest || !reflect.DeepEqual(report.Schemas[0].Fields, previous.Fields)) {
			t.Fatalf("constraint-only change must alter full digest while preserving Access view: before=%#v after=%#v", previous, report.Schemas[0])
		}
		previous = report.Schemas[0]
	}
}

func TestSchemaDescriptionDigestCoversConstraintOnlyChanges(t *testing.T) {
	const id objectmodel.ObjectID = "schema/order.definition"
	before, err := reader.InspectSchemaValue(id, constrainedDescriptionSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{name: "required", change: func(value map[string]any) {
			value["fields"].(map[string]any)["target"].(map[string]any)["required"] = false
		}},
		{name: "reference_types", change: func(value map[string]any) {
			value["fields"].(map[string]any)["target"].(map[string]any)["refTypes"] = []any{"Module"}
		}},
		{name: "description", change: func(value map[string]any) { value["description"] = "Changed description" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := constrainedDescriptionSchema()
			tc.change(value)
			after, err := reader.InspectSchemaValue(id, value)
			if err != nil {
				t.Fatal(err)
			}
			if after.Digest == before.Digest || !reflect.DeepEqual(after.Fields, before.Fields) {
				t.Fatalf("full Schema digest must cover constraints omitted from compatibility Access view: before=%#v after=%#v", before, after)
			}
		})
	}
}
