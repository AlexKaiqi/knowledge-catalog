package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestSchemaStructureDigestExcludesOnlyFieldAccessAndPreservesOriginal(t *testing.T) {
	base := knowledge.SchemaDefinition{ObjectID: "schema/implements.example/v1", MetaSchema: knowledge.MetaSchemaV1, Entity: "Object", Description: "Implementation", Aspect: "interfaces", Pattern: "keyed_collection", Origin: "https://runtime.example", Fields: []knowledge.SchemaFieldDefinition{{Path: "target", Type: "object_ref", Required: true, Access: []string{"filter"}, RefTypes: []string{"Object"}}}}
	originalDigest := kernel.CanonicalDigest(base)
	structureDigest := knowledge.SchemaStructureDigest(base)
	withoutAccess := base
	withoutAccess.Fields = append([]knowledge.SchemaFieldDefinition(nil), base.Fields...)
	withoutAccess.Fields[0].Access = nil
	if structureDigest != knowledge.SchemaStructureDigest(withoutAccess) || originalDigest == kernel.CanonicalDigest(withoutAccess) {
		t.Fatal("field access must change full schema evidence but not support matching")
	}
	if kernel.CanonicalDigest(base) != originalDigest || len(base.Fields[0].Access) != 1 {
		t.Fatal("support matching mutated the original schema")
	}
	changes := map[string]func(*knowledge.SchemaDefinition){
		"identity":             func(s *knowledge.SchemaDefinition) { s.ObjectID = "schema/implements.example/v2" },
		"meta schema":          func(s *knowledge.SchemaDefinition) { s.MetaSchema = "schema/schema-definition/v2" },
		"entity":               func(s *knowledge.SchemaDefinition) { s.Entity = "Table" },
		"description":          func(s *knowledge.SchemaDefinition) { s.Description = "Changed" },
		"aspect":               func(s *knowledge.SchemaDefinition) { s.Aspect = "bindings" },
		"pattern":              func(s *knowledge.SchemaDefinition) { s.Pattern = "record" },
		"additionalProperties": func(s *knowledge.SchemaDefinition) { s.AdditionalProperties = true },
		"origin":               func(s *knowledge.SchemaDefinition) { s.Origin = "https://other.example" },
		"field path":           func(s *knowledge.SchemaDefinition) { s.Fields[0].Path = "source" },
		"field type":           func(s *knowledge.SchemaDefinition) { s.Fields[0].Type = "string" },
		"required":             func(s *knowledge.SchemaDefinition) { s.Fields[0].Required = false },
		"refTypes":             func(s *knowledge.SchemaDefinition) { s.Fields[0].RefTypes = []string{"Table"} },
		"field removal":        func(s *knowledge.SchemaDefinition) { s.Fields = nil },
		"field addition": func(s *knowledge.SchemaDefinition) {
			s.Fields = append(s.Fields, knowledge.SchemaFieldDefinition{Path: "name", Type: "string"})
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Fields = append([]knowledge.SchemaFieldDefinition(nil), base.Fields...)
			change(&changed)
			if knowledge.SchemaStructureDigest(changed) == structureDigest {
				t.Fatal("support matching discarded a non-access schema semantic")
			}
		})
	}
}

func TestSchemaStructureDigestPreservesEmptyFieldRepresentation(t *testing.T) {
	for _, fields := range [][]knowledge.SchemaFieldDefinition{nil, {}} {
		definition := knowledge.SchemaDefinition{ObjectID: "schema/empty/v1", Entity: "Object", Pattern: "record", Fields: fields}
		if knowledge.SchemaStructureDigest(definition) != kernel.CanonicalDigest(definition) {
			t.Fatal("schema without access declarations changed during support matching")
		}
	}
}
