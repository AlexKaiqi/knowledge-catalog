package object_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestSchemaStructureDigestCoversEverySchemaSemantic(t *testing.T) {
	base := objectmodel.SchemaDefinition{ObjectID: "schema/implements.example/v1", MetaSchema: objectmodel.MetaSchemaV1, Entity: "Object", Description: "Implementation", Aspect: "interfaces", Pattern: "keyed_collection", Fields: []objectmodel.SchemaFieldDefinition{{Path: "target", Type: "object_ref", Required: true, RefTypes: []string{"Object"}}}}
	structureDigest := objectmodel.SchemaStructureDigest(base)
	if structureDigest != kernel.CanonicalDigest(base) {
		t.Fatal("a structure-only Schema must match on its complete definition")
	}
	changes := map[string]func(*objectmodel.SchemaDefinition){
		"identity":             func(s *objectmodel.SchemaDefinition) { s.ObjectID = "schema/implements.example/v2" },
		"meta schema":          func(s *objectmodel.SchemaDefinition) { s.MetaSchema = "schema/schema-definition/v2" },
		"entity":               func(s *objectmodel.SchemaDefinition) { s.Entity = "Table" },
		"description":          func(s *objectmodel.SchemaDefinition) { s.Description = "Changed" },
		"aspect":               func(s *objectmodel.SchemaDefinition) { s.Aspect = "bindings" },
		"pattern":              func(s *objectmodel.SchemaDefinition) { s.Pattern = "record" },
		"additionalProperties": func(s *objectmodel.SchemaDefinition) { s.AdditionalProperties = true },
		"field path":           func(s *objectmodel.SchemaDefinition) { s.Fields[0].Path = "source" },
		"field type":           func(s *objectmodel.SchemaDefinition) { s.Fields[0].Type = "string" },
		"required":             func(s *objectmodel.SchemaDefinition) { s.Fields[0].Required = false },
		"refTypes":             func(s *objectmodel.SchemaDefinition) { s.Fields[0].RefTypes = []string{"Table"} },
		"field removal":        func(s *objectmodel.SchemaDefinition) { s.Fields = nil },
		"field addition": func(s *objectmodel.SchemaDefinition) {
			s.Fields = append(s.Fields, objectmodel.SchemaFieldDefinition{Path: "name", Type: "string"})
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Fields = append([]objectmodel.SchemaFieldDefinition(nil), base.Fields...)
			change(&changed)
			if objectmodel.SchemaStructureDigest(changed) == structureDigest {
				t.Fatal("support matching discarded a schema semantic")
			}
		})
	}
}

func TestSchemaStructureDigestPreservesEmptyFieldRepresentation(t *testing.T) {
	for _, fields := range [][]objectmodel.SchemaFieldDefinition{nil, {}} {
		definition := objectmodel.SchemaDefinition{ObjectID: "schema/empty/v1", Entity: "Object", Pattern: "record", Fields: fields}
		if objectmodel.SchemaStructureDigest(definition) != kernel.CanonicalDigest(definition) {
			t.Fatal("schema changed during support matching")
		}
	}
}
