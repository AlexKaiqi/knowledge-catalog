package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
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
