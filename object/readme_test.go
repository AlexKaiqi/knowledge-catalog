package object_test

import (
	"kc/internal/testkit"
	"maps"
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

func TestSystemRepositoryPublishesReadmeSchemaAndInstance(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(objectmodel.ObjectID("schema/core/readme/v1"), head)
	if err != nil {
		t.Fatal(err)
	}
	report, err := objectmodel.ParseSchemaDefinition(objectmodel.ObjectID("schema/core/readme/v1"), value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entity != "Readme" || report.Aspect != "readme" || report.Pattern != "record" || report.AdditionalProperties || report.Description == "" {
		t.Fatalf("%#v", report)
	}
	readme, err := repo.Read(objectmodel.ObjectID("kc/system"), head)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := testkit.MarkdownBody(readme.Value)
	if !ok || !strings.Contains(body, "KC 协议仓") {
		t.Fatalf("system readme value %#v", readme.Value)
	}
	files, ok := any(repo).(objectmodel.ObjectFileReader)
	if !ok {
		t.Fatal("SystemRepository must expose README.md")
	}
	raw, err := files.ReadObjectFile("README.md", head)
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

func TestAssertProtocolSchemaPublication(t *testing.T) {
	published := map[objectmodel.ObjectID]bool{}
	for _, operation := range objectmodel.TrustedSchemaOperations() {
		id := operation.Address.ObjectID
		published[id] = true
		t.Run(string(id), func(t *testing.T) {
			assertProtocolSchemaCopy(t, operation)
		})
	}
	for _, id := range []objectmodel.ObjectID{objectmodel.MetaSchemaV1, objectmodel.IndexDeclarationSchemaV1} {
		if !published[id] {
			t.Fatalf("protocol Schema %s is absent from the trust root", id)
		}
	}
	drifted := map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Other", "pattern": "record",
		"additionalProperties": false,
		"fields":               map[string]any{"body": map[string]any{"type": "string", "required": true}},
	}
	if err := objectmodel.AssertProtocolSchemaPublication(objectmodel.IndexDeclarationSchemaV1, drifted); err == nil {
		t.Fatal("drifted protocol schema must fail")
	}
	// readme and relation are ordinary content Schemas: a Repository may
	// publish its own version under the same object ID.
	for _, id := range []objectmodel.ObjectID{"schema/core/readme/v1", objectmodel.CoreRelationSchemaV1} {
		if err := objectmodel.AssertProtocolSchemaPublication(id, drifted); err != nil {
			t.Fatalf("%s is not a protocol schema: %v", id, err)
		}
	}
}

func assertProtocolSchemaCopy(t *testing.T, operation objectmodel.Operation) {
	t.Helper()
	id := operation.Address.ObjectID
	if err := objectmodel.AssertProtocolSchemaPublication(id, operation.Value); err != nil {
		t.Fatalf("identical protocol Schema copy was rejected: %v", err)
	}
	body, ok := operation.Value.(map[string]any)
	if !ok {
		t.Fatalf("protocol Schema has unexpected value %T", operation.Value)
	}
	drifted := maps.Clone(body)
	description, _ := body["description"].(string)
	drifted["description"] = description + " 本地修改的说明。"
	previous, err := objectmodel.ParseSchemaDefinition(id, body)
	if err != nil {
		t.Fatal(err)
	}
	next, err := objectmodel.ParseSchemaDefinition(id, drifted)
	if err != nil {
		t.Fatalf("description-only drift must still be a valid Schema: %v", err)
	}
	if changes := objectmodel.BreakingSchemaChanges(previous, next); len(changes) != 0 {
		t.Fatalf("description-only drift must not depend on structural incompatibility: %v", changes)
	}
	testkit.ExpectCode(t, objectmodel.AssertProtocolSchemaPublication(id, drifted), kernel.ErrSchemaIncompatible)
}

func TestReadmeSchemaIsNotInTheTrustRoot(t *testing.T) {
	for _, operation := range objectmodel.TrustedSchemaOperations() {
		if operation.Address.ObjectID == "schema/core/readme/v1" {
			t.Fatal("readme is an ordinary Schema published by the System Repository, not protocol trust root")
		}
	}
}
