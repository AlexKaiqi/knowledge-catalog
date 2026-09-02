package writer_test

import (
	"bytes"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

func TestPublishSystemSeedsEmptyTreeAndRefusesOverwrite(t *testing.T) {
	empty := testkit.MakeRepository(t, "kr://acme/public/core")
	if _, err := writer.PublishSystem(empty.Snapshot()); err == nil {
		t.Fatal("publish must refuse a non-system repository")
	} else if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("wrong code: %v", err)
	}

	native := knowledge.NewSystemRepository()
	verified, err := writer.PublishSystem(native)
	if err != nil {
		t.Fatal(err)
	}
	head, err := native.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Seeded || verified.Commit != head {
		t.Fatalf("built-in System Repository must verify without writing: %#v", verified)
	}

	repo := testkit.MakeRepository(t, string(knowledge.SystemRepositoryID))
	before, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := writer.PublishSystem(repo.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !seeded.Seeded || seeded.Commit == "" || seeded.Commit == before {
		t.Fatalf("empty system authority must be seeded: %#v", seeded)
	}
	replay, err := writer.PublishSystem(repo.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if replay.Seeded || replay.Commit != seeded.Commit {
		t.Fatalf("matching publication must verify in place: seeded %#v replay %#v", seeded, replay)
	}

	tree, ok := snapshot.TreeStoreOf(repo.Snapshot())
	if !ok {
		t.Fatal("memory fixture must expose TreeStore")
	}
	files, err := tree.ListFiles(seeded.Commit)
	if err != nil {
		t.Fatal(err)
	}
	path := ""
	for _, file := range files {
		if strings.HasPrefix(file, ".kc/") {
			continue
		}
		if !strings.HasPrefix(file, "schemas/") {
			t.Fatalf("system publication must use the schemas/ tree, got %s", file)
		}
		if strings.Contains(file, "schema-definition") {
			path = file
		}
	}
	if path != "schemas/schema-definition.v1.aspect.yaml" {
		t.Fatalf("seeded tree missing meta schema file: %v", files)
	}
	raw, err := tree.ReadFile(path, seeded.Commit)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte("SchemaDefinition"), []byte("TamperedSchema"), 1)
	if _, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     knowledge.SystemRepositoryID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           seeded.Commit,
		ExpectedTargetCommit: seeded.Commit,
		Changes:              []snapshot.TreeChange{{Path: path, Content: tampered}},
		Message:              "tamper system schema",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.PublishSystem(repo.Snapshot()); err == nil {
		t.Fatal("publish must refuse a mismatched System Schema")
	} else if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("wrong code: %v", err)
	}
}
