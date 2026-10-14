package systemrepo_test

import (
	"bytes"
	"kc/object/writer"
	"kc/systemrepo"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

func TestPublishSystemSeedsEmptyTreeAndRefusesOverwrite(t *testing.T) {
	empty := testkit.MakeRepository(t, "kr://acme/public/core")
	if _, err := systemrepo.PublishSystem(empty.Snapshot()); err == nil {
		t.Fatal("publish must refuse a non-system repository")
	} else if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("wrong code: %v", err)
	}

	native := testkit.MakeNativeSystemRepository(t)
	verified, err := systemrepo.PublishSystem(native)
	if err != nil {
		t.Fatal(err)
	}
	head, err := native.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Seeded || verified.Commit != head {
		t.Fatalf("published native System Repository must verify without writing: %#v", verified)
	}

	repo := testkit.MakeRepository(t, string(objectmodel.SystemRepositoryID))
	before, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := systemrepo.PublishSystem(repo.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !seeded.Seeded || seeded.Commit == "" || seeded.Commit == before {
		t.Fatalf("empty system authority must be seeded: %#v", seeded)
	}
	replay, err := systemrepo.PublishSystem(repo.Snapshot())
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
	hasReadme := false
	expectedFiles := map[string]bool{}
	for _, operation := range systemrepo.SystemPublicationOperations() {
		expectedFiles[operation.PathHint] = false
	}
	for _, file := range files {
		if strings.HasPrefix(file, ".kc/") {
			continue
		}
		if _, exists := expectedFiles[file]; !exists {
			t.Fatalf("unexpected file in system publication: %s", file)
		}
		expectedFiles[file] = true
		if file == objectmodel.RepositoryReadmePath {
			hasReadme = true
			continue
		}
		raw, err := tree.ReadFile(file, seeded.Commit)
		if err != nil {
			t.Fatal(err)
		}
		unit := repofile.Parse(string(raw))
		if unit == nil {
			t.Fatalf("published file is not an object unit: %s", file)
		}
		if objectmodel.IsSchemaObject(unit.ObjectID) && !strings.HasPrefix(file, objectmodel.CanonicalSchemaDir+"/") {
			t.Fatalf("system publication must use the _schemas/ tree, got %s", file)
		}
		if strings.Contains(file, "schema-definition") {
			path = file
		}
	}
	for file, present := range expectedFiles {
		if !present {
			t.Errorf("system publication omitted %s", file)
		}
	}
	if !hasReadme {
		t.Fatalf("seeded tree missing README.md: %v", files)
	}
	if path != objectmodel.CanonicalSchemaDir+"/schema-definition.v1.aspect.yaml" {
		t.Fatalf("seeded tree missing meta schema file: %v", files)
	}
	raw, err := tree.ReadFile(path, seeded.Commit)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte("SchemaDefinition"), []byte("TamperedSchema"), 1)
	if _, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     objectmodel.SystemRepositoryID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           seeded.Commit,
		ExpectedTargetCommit: seeded.Commit,
		Changes:              []snapshot.TreeChange{{Path: path, Content: tampered}},
		Message:              "tamper system schema",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := systemrepo.PublishSystem(repo.Snapshot()); err == nil {
		t.Fatal("publish must refuse a mismatched System Schema")
	} else if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestPublishSystemRejectsSelfDescriptionDrift(t *testing.T) {
	for _, tc := range []struct {
		name, path, before, after string
		remove                    bool
	}{
		{"readme", "README.md", "Knowledge Catalog", "Altered Catalog", false},
		{"concept", "concepts/object/definition.md", "具有独立身份", "没有独立身份", false},
		{"entrypoint", "capabilities/read-object/entrypoints.yaml", "kc read", "kc invented", false},
		{"schema binding", "capabilities/read-object/definition.md", "capability-definition/v1", "capability-usage/v1", false},
		{"missing guide", "guides/first-publication/guide.md", "", "", true},
		{"missing relation", "relations/first-publication.uses.publish-object.yaml", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testkit.MakeRepository(t, string(objectmodel.SystemRepositoryID))
			published, err := systemrepo.PublishSystem(repo.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			tree, _ := snapshot.TreeStoreOf(repo.Snapshot())
			raw, err := tree.ReadFile(tc.path, published.Commit)
			if err != nil {
				t.Fatal(err)
			}
			altered := bytes.Replace(raw, []byte(tc.before), []byte(tc.after), 1)
			if !tc.remove && bytes.Equal(altered, raw) {
				t.Fatal("tamper signal did not change the publication")
			}
			commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: objectmodel.SystemRepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: published.Commit, ExpectedTargetCommit: published.Commit, Changes: []snapshot.TreeChange{{Path: tc.path, Content: altered, Remove: tc.remove}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := systemrepo.PublishSystem(repo.Snapshot()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("publication drift was accepted: %v", err)
			}
			if head, _ := repo.Head(snapshot.DefaultRef); head != commit {
				t.Fatal("verification overwrote occupied publication")
			}
		})
	}
}

func TestRuntimeWriterRefusesSystemRepository(t *testing.T) {
	w, err := writer.NewWriter(snapshot.NewRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("mutate-system", objectmodel.CommitChangeSet{
		TargetRepository: objectmodel.SystemRepositoryID,
		TargetRef:        snapshot.DefaultRef,
		Operations:       []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/evil"}, Value: map[string]any{"entity": "Evil"}}},
	})
	if kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("runtime Writer must refuse System Repository: %v", err)
	}
}

func TestPublishSystemRejectsUnexpectedFiles(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		repo := testkit.MakeRepository(t, string(objectmodel.SystemRepositoryID))
		if seeded {
			if _, err := systemrepo.PublishSystem(repo.Snapshot()); err != nil {
				t.Fatal(err)
			}
		}
		tree, _ := snapshot.TreeStoreOf(repo.Snapshot())
		base, _ := repo.Head(snapshot.DefaultRef)
		commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: objectmodel.SystemRepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Changes: []snapshot.TreeChange{{Path: "foreign.txt", Content: []byte("unrecognized publication")}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := systemrepo.PublishSystem(repo.Snapshot()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("occupied publication accepted extra files (seeded=%v): %v", seeded, err)
		}
		if head, _ := repo.Head(snapshot.DefaultRef); head != commit {
			t.Fatal("occupied publication was overwritten")
		}
	}
}
