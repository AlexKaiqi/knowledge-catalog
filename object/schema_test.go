package object_test

import (
	"kc/internal/testkit"
	"kc/systemrepo"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func TestSystemRepositoryPublishesTrustedMetaSchema(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	if repo.ID() != objectmodel.SystemRepositoryID {
		t.Fatalf("system repository id %s", repo.ID())
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(objectmodel.MetaSchemaV1, head)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kernel.CanonicalDigest(value.Value), objectmodel.SystemMetaSchemaDigest(); got != want {
		t.Fatalf("meta schema digest %s, want %s", got, want)
	}
	report, err := objectmodel.ParseSchemaDefinition(objectmodel.MetaSchemaV1, value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.MetaSchema != objectmodel.MetaSchemaV1 || report.Entity != "SchemaDefinition" {
		t.Fatalf("unexpected meta schema %#v", report)
	}
	foundDescription := false
	for _, field := range report.Fields {
		if field.Path == "description" {
			foundDescription = true
			break
		}
	}
	if !foundDescription {
		t.Fatal("meta schema must declare optional entity description")
	}
}

func TestEmbeddedSystemSchemasParseAsDomainSchema(t *testing.T) {
	for _, operation := range systemrepo.SystemSchemaOperations() {
		if _, err := objectmodel.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value); err != nil {
			t.Fatalf("%s: %v", operation.Address.ObjectID, err)
		}
		if operation.PathHint != repofile.DefaultSchemaPath(operation.Address.ObjectID) {
			t.Fatalf("%s path hint %s, want Canonical _schemas/ path", operation.Address.ObjectID, operation.PathHint)
		}
	}
}

func TestSchemaExamplesIngestAndDescribe(t *testing.T) {
	dir := schemaExamplesDir(t)
	preview, err := writer.Ingest(dir, "kr://acme/public/core", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChangeSet.Operations) != 5 {
		t.Fatalf("operations=%d files=%d", len(preview.ChangeSet.Operations), len(preview.Files))
	}

	var instance *objectmodel.Operation
	schemas := 0
	for i := range preview.ChangeSet.Operations {
		op := preview.ChangeSet.Operations[i]
		if objectmodel.IsSchemaObject(op.Address.ObjectID) {
			schemas++
			if _, err := objectmodel.ParseSchemaDefinition(op.Address.ObjectID, op.Value); err != nil {
				t.Fatalf("%s: %v", op.Address.ObjectID, err)
			}
			desc, err := reader.InspectSchemaValue(op.Address.ObjectID, op.Value)
			if err != nil {
				t.Fatalf("%s: %v", op.Address.ObjectID, err)
			}
			if desc.Pattern != "record" || desc.Entity == "" || desc.Aspect == "" {
				t.Fatalf("%s: %#v", op.Address.ObjectID, desc)
			}
			continue
		}
		instance = &preview.ChangeSet.Operations[i]
	}
	if schemas != 4 {
		t.Fatalf("schema objects=%d", schemas)
	}
	if instance == nil || instance.Address.ObjectID != "runbook/payment-oncall" {
		t.Fatalf("instance %#v", instance)
	}
	if instance.SchemaRef != "schema/runbook.body" {
		t.Fatalf("schema_ref %q", instance.SchemaRef)
	}
	for _, op := range preview.ChangeSet.Operations {
		if !objectmodel.IsSchemaObject(op.Address.ObjectID) {
			continue
		}
		if op.PathHint != repofile.DefaultSchemaPath(op.Address.ObjectID) {
			t.Fatalf("%s path hint %s", op.Address.ObjectID, op.PathHint)
		}
	}

	unit := repofile.Parse(mustRead(t, filepath.Join(dir, "runbooks", "runbook.payment-oncall.yaml")))
	if unit == nil || unit.SchemaRef != "schema/runbook.body" {
		t.Fatalf("unit %#v", unit)
	}
}

func schemaExamplesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "schema-examples")
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSystemRepositoryHistory(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	revisions, err := repo.Log(objectmodel.MetaSchemaV1, head, objectmodel.ObjectLogQuery{})
	if err != nil || len(revisions) != 1 {
		t.Fatalf("system log %#v err=%v", revisions, err)
	}
	after, err := repo.Log(objectmodel.MetaSchemaV1, head, objectmodel.ObjectLogQuery{After: revisions[0].Commit})
	if err != nil || len(after) != 0 {
		t.Fatalf("After must exhaust the single system revision: %#v err=%v", after, err)
	}
}

func TestSchemaCompatibilitySeparatesAccessFromValueStructure(t *testing.T) {
	previous := objectmodel.SchemaDefinition{ObjectID: "schema/article", Entity: "Article", Pattern: "record", Fields: []objectmodel.SchemaFieldDefinition{{Path: "body", Type: "string", Access: []string{"text", "filter"}}}}
	for _, access := range [][]string{{"filter"}, {}} {
		next := previous
		next.Fields = []objectmodel.SchemaFieldDefinition{{Path: "body", Type: "string", Access: access}}
		if changes := objectmodel.BreakingSchemaChanges(previous, next); len(changes) > 0 {
			t.Fatalf("access-only change is not a value break: %v", changes)
		}
		next.Fields[0].Type = "integer"
		if changes := objectmodel.BreakingSchemaChanges(previous, next); len(changes) == 0 {
			t.Fatal("removing query access must not permit a value type change")
		}
	}
}
