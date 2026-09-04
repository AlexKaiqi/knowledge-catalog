package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func TestSystemRepositoryPublishesSourceProfileSchema(t *testing.T) {
	repo := knowledge.NewSystemRepository()
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(knowledge.CoreSourceProfileSchemaV1, head)
	if err != nil {
		t.Fatal(err)
	}
	report, err := knowledge.ParseSchemaDefinition(knowledge.CoreSourceProfileSchemaV1, value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entity != "SourceProfile" || report.Pattern != "record" || report.AdditionalProperties {
		t.Fatalf("unexpected source profile schema %#v", report)
	}
	got := map[string]knowledge.SchemaFieldDefinition{}
	for _, field := range report.Fields {
		got[field.Path] = field
	}
	if !got["title"].Required || !got["summary"].Required {
		t.Fatalf("source profile envelope must require title and summary: %#v", report.Fields)
	}
}

func TestAssertSourceProfileBinding(t *testing.T) {
	reserved := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.SourceProfileObjectID}
	if err := knowledge.AssertSourceProfileBinding(reserved, knowledge.CoreSourceProfileSchemaV1); err != nil {
		t.Fatal(err)
	}
	if err := knowledge.AssertSourceProfileBinding(reserved, ""); err == nil {
		t.Fatal("reserved object without the protocol schema must fail")
	}
	other := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "core/source-profile-extra"}
	if err := knowledge.AssertSourceProfileBinding(other, knowledge.CoreSourceProfileSchemaV1); err == nil {
		t.Fatal("non-reserved object must not adopt the protocol schema")
	}
	aspect := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.SourceProfileObjectID, AspectName: "body"}
	if err := knowledge.AssertSourceProfileBinding(aspect, knowledge.CoreSourceProfileSchemaV1); err == nil {
		t.Fatal("source profile must be an entity address")
	}
}

func TestAssertProtocolSchemaPublication(t *testing.T) {
	var published any
	for _, operation := range knowledge.SystemSchemaOperations() {
		if operation.Address.ObjectID == knowledge.CoreSourceProfileSchemaV1 {
			published = operation.Value
			break
		}
	}
	if published == nil {
		t.Fatal("source profile schema is not published")
	}
	if err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreSourceProfileSchemaV1, published); err != nil {
		t.Fatal(err)
	}
	drifted := map[string]any{
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "SourceProfile", "pattern": "record",
		"additionalProperties": false,
		"fields": map[string]any{
			"title":   map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"summary": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
		},
	}
	err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreSourceProfileSchemaV1, drifted)
	if err == nil {
		t.Fatal("drifted protocol schema must fail")
	}
	if kernel.CodeOf(err) != kernel.ErrSchemaIncompatible {
		t.Fatalf("code=%s", kernel.CodeOf(err))
	}
	if err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreRelationSchemaV1, drifted); err != nil {
		t.Fatal(err)
	}
}
