package knowledge_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

func TestSystemRepositoryPublishesTrustedMetaSchema(t *testing.T) {
	repo := knowledge.NewSystemRepository()
	if repo.ID() != knowledge.SystemRepositoryID {
		t.Fatalf("system repository id %s", repo.ID())
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read(knowledge.MetaSchemaV1, head)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kernel.CanonicalDigest(value.Value), knowledge.SystemMetaSchemaDigest(); got != want {
		t.Fatalf("meta schema digest %s, want %s", got, want)
	}
	report, err := knowledge.ParseSchemaDefinition(knowledge.MetaSchemaV1, value.Value)
	if err != nil {
		t.Fatal(err)
	}
	if report.MetaSchema != knowledge.MetaSchemaV1 || report.Entity != "SchemaDefinition" {
		t.Fatalf("unexpected meta schema %#v", report)
	}
}

func TestEmbeddedSystemSchemasParseAsDomainSchema(t *testing.T) {
	for _, operation := range knowledge.SystemSchemaOperations() {
		if _, err := knowledge.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value); err != nil {
			t.Fatalf("%s: %v", operation.Address.ObjectID, err)
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

	var instance *knowledge.Operation
	schemas := 0
	for i := range preview.ChangeSet.Operations {
		op := preview.ChangeSet.Operations[i]
		if knowledge.IsSchemaObject(op.Address.ObjectID) {
			schemas++
			if _, err := knowledge.ParseSchemaDefinition(op.Address.ObjectID, op.Value); err != nil {
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

	unit := repofile.Parse(mustRead(t, filepath.Join(dir, "objects", "runbook.payment-oncall.okf")))
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

func TestSystemRepositoryIsImmutable(t *testing.T) {
	repo := knowledge.NewSystemRepository()
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRef("refs/heads/other", head); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("CreateRef error %v", err)
	}
	if err := repo.Archive(); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("Archive error %v", err)
	}
}
