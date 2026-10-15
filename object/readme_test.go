package object_test

import (
	"kc/internal/testkit"
	"kc/systemrepo"
	"strings"
	"testing"

	objectmodel "kc/object"
	repofile "kc/object/format"
)

func TestSystemRepositoryPublishesReadmeSchemaAndInstance(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(objectmodel.CoreReadmeSchemaV1, head)
	if err != nil {
		t.Fatal(err)
	}
	report, err := objectmodel.ParseSchemaDefinition(objectmodel.CoreReadmeSchemaV1, value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entity != "Readme" || report.Aspect != objectmodel.ReadmeAspect || report.Pattern != "record" || report.AdditionalProperties || report.Description == "" {
		t.Fatalf("%#v", report)
	}
	readme, err := repo.Read(objectmodel.SystemReadmeObjectID, head)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := objectmodel.ReadmeBody(readme.Value)
	if !ok || !strings.Contains(body, "KC 协议仓") {
		t.Fatalf("system readme value %#v", readme.Value)
	}
	files, ok := any(repo).(objectmodel.ObjectFileReader)
	if !ok {
		t.Fatal("SystemRepository must expose README.md")
	}
	raw, err := files.ReadObjectFile(objectmodel.RepositoryReadmePath, head)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("published README.md is empty")
	}
	schemaYAML, err := files.ReadObjectFile("_schemas/readme.v1.aspect.yaml", head)
	if err != nil {
		t.Fatal(err)
	}
	unit := repofile.Parse(string(schemaYAML))
	if unit == nil || unit.Value.(map[string]any)["entity"] != "Readme" {
		t.Fatalf("system schema file is missing: %s", schemaYAML)
	}
}

func TestAssertReadmeBinding(t *testing.T) {
	okAddr := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: objectmodel.ReadmeAspect}
	if err := objectmodel.AssertReadmeBinding(okAddr, objectmodel.CoreReadmeSchemaV1); err != nil {
		t.Fatal(err)
	}
	blob := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/payments"}
	if err := objectmodel.AssertReadmeBinding(blob, objectmodel.CoreReadmeSchemaV1); err == nil {
		t.Fatal("entity blob must not use the README schema")
	}
	other := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: "body"}
	if err := objectmodel.AssertReadmeBinding(other, objectmodel.CoreReadmeSchemaV1); err == nil {
		t.Fatal("wrong aspect must not use the README schema")
	}
	if err := objectmodel.AssertReadmeBinding(okAddr, ""); err == nil {
		t.Fatal("readme aspect requires the protocol schema")
	}
}

func TestAssertProtocolSchemaPublication(t *testing.T) {
	var published any
	for _, operation := range systemrepo.SystemSchemaOperations() {
		if operation.Address.ObjectID == objectmodel.CoreReadmeSchemaV1 {
			published = operation.Value
			break
		}
	}
	if published == nil {
		t.Fatal("readme schema is not published")
	}
	if err := objectmodel.AssertProtocolSchemaPublication(objectmodel.CoreReadmeSchemaV1, published); err != nil {
		t.Fatal(err)
	}
	drifted := map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Other", "aspect": objectmodel.ReadmeAspect, "pattern": "record",
		"additionalProperties": false,
		"fields":               map[string]any{"body": map[string]any{"type": "string", "required": true, "access": []any{"text"}}},
	}
	err := objectmodel.AssertProtocolSchemaPublication(objectmodel.CoreReadmeSchemaV1, drifted)
	if err == nil {
		t.Fatal("drifted protocol schema must fail")
	}
	if err := objectmodel.AssertProtocolSchemaPublication(objectmodel.CoreRelationSchemaV1, drifted); err != nil {
		t.Fatal(err)
	}
}
