package knowledge_test

import (
	"strings"
	"testing"

	"kc/knowledge"
)

func TestSystemRepositoryPublishesReadmeSchemaAndInstance(t *testing.T) {
	repo := knowledge.NewSystemRepository()
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(knowledge.CoreReadmeSchemaV1, head)
	if err != nil {
		t.Fatal(err)
	}
	report, err := knowledge.ParseSchemaDefinition(knowledge.CoreReadmeSchemaV1, value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entity != "Readme" || report.Aspect != knowledge.ReadmeAspect || report.Pattern != "record" || report.AdditionalProperties || report.Description == "" {
		t.Fatalf("%#v", report)
	}
	readme, err := repo.Read(knowledge.SystemReadmeObjectID, head)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := knowledge.ReadmeBody(readme.Value)
	if !ok || !strings.Contains(body, "KC 协议仓") {
		t.Fatalf("system readme value %#v", readme.Value)
	}
	files, ok := any(repo).(knowledge.KnowledgeFileReader)
	if !ok {
		t.Fatal("SystemRepository must expose README.md")
	}
	raw, err := files.ReadKnowledgeFile(knowledge.RepositoryReadmePath, head)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("embedded README.md is empty")
	}
	schemaYAML, err := files.ReadKnowledgeFile("_schemas/readme.v1.aspect.yaml", head)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schemaYAML), "entity: Readme") {
		t.Fatalf("system schema file is missing: %s", schemaYAML)
	}
}

func TestAssertReadmeBinding(t *testing.T) {
	okAddr := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "payments", AspectName: knowledge.ReadmeAspect}
	if err := knowledge.AssertReadmeBinding(okAddr, knowledge.CoreReadmeSchemaV1); err != nil {
		t.Fatal(err)
	}
	blob := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "payments"}
	if err := knowledge.AssertReadmeBinding(blob, knowledge.CoreReadmeSchemaV1); err == nil {
		t.Fatal("entity blob must not use the README schema")
	}
	other := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "payments", AspectName: "body"}
	if err := knowledge.AssertReadmeBinding(other, knowledge.CoreReadmeSchemaV1); err == nil {
		t.Fatal("wrong aspect must not use the README schema")
	}
	if err := knowledge.AssertReadmeBinding(okAddr, ""); err == nil {
		t.Fatal("readme aspect requires the protocol schema")
	}
}

func TestAssertProtocolSchemaPublication(t *testing.T) {
	var published any
	for _, operation := range knowledge.SystemSchemaOperations() {
		if operation.Address.ObjectID == knowledge.CoreReadmeSchemaV1 {
			published = operation.Value
			break
		}
	}
	if published == nil {
		t.Fatal("readme schema is not published")
	}
	if err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreReadmeSchemaV1, published); err != nil {
		t.Fatal(err)
	}
	drifted := map[string]any{
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "Other", "aspect": knowledge.ReadmeAspect, "pattern": "record",
		"additionalProperties": false,
		"fields":               map[string]any{"body": map[string]any{"type": "string", "required": true, "access": []any{"text"}}},
	}
	err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreReadmeSchemaV1, drifted)
	if err == nil {
		t.Fatal("drifted protocol schema must fail")
	}
	if err := knowledge.AssertProtocolSchemaPublication(knowledge.CoreRelationSchemaV1, drifted); err != nil {
		t.Fatal(err)
	}
}
