package writer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func TestDirectoryIdentityChangeCannotOverwriteUnselectedPathOwner(t *testing.T) {
	f := newRepresentationFixture(t)
	drafts := t.TempDir()
	path := filepath.Join(drafts, "records", "orders.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name: orders\n"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(drafts, f.store.ID(), testkit.MustHead(t, f.store, snapshot.DefaultRef))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files[0].IdentitySource != "path" || preview.Files[0].ObjectID != "records/orders" {
		t.Fatalf("first publication must reproduce implicit path identity: %#v", preview.Files)
	}
	first, err := f.writer.Commit("implicit-identity", preview.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	before := first.Result.NewCommit
	content := "---\nobject_id: table/orders\nkind: Aspect\naspect_name: properties\n---\nname: orders\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := writer.Ingest(drafts, f.store.ID(), before)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := f.writer.Commit("explicit-identity-without-remove", changed.ChangeSet)
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Errorf("same path must not replace its unselected identity, got %v and %#v", err, receipt)
	}
	if err != nil && (!strings.Contains(err.Error(), "records/orders.yaml") || !strings.Contains(err.Error(), "records/orders") || !strings.Contains(err.Error(), "REMOVE")) {
		t.Errorf("path conflict must name its owner and explicit recovery: %v", err)
	}
	if head := testkit.MustHead(t, f.store, snapshot.DefaultRef); head != before {
		t.Errorf("rejected path takeover advanced HEAD: %s -> %s", before, head)
	}
	if receipt.Result.NewCommit != "" || receipt.Disposition == writer.DispositionApplied {
		t.Errorf("rejected path takeover returned an accepted receipt: %#v", receipt)
	}
	if saved, ok := f.writer.Lookup("explicit-identity-without-remove"); ok && saved.Receipt.Disposition == writer.DispositionApplied {
		t.Errorf("rejected path takeover left an accepted durable receipt: %#v", saved)
	}
	original, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: "records/orders"}, before, nil)
	if err != nil || original.Value != "name: orders\n" {
		t.Fatalf("old fixed commit changed: %#v %v", original, err)
	}
	if _, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: "table/orders"}, before, nil); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("new identity appeared in the old fixed commit: %v", err)
	}
	// Updating the same directory identity at its existing path stays legal.
	if err := os.WriteFile(path, []byte("name: revised orders\n"), 0600); err != nil {
		t.Fatal(err)
	}
	update, err := writer.Ingest(drafts, f.store.ID(), before)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.writer.Commit("same-directory-identity-update", update.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: "records/orders"}, accepted.Result.NewCommit, nil)
	if err != nil || updated.Value != "name: revised orders\n" {
		t.Fatalf("legal same-identity directory update changed behavior: %#v %v", updated, err)
	}
}

func TestExplicitPathOwnerRemovalKeepsFixedIdentityAndDatasetScope(t *testing.T) {
	for _, putFirst := range []bool{false, true} {
		name := "remove_then_put"
		if putFirst {
			name = "put_then_remove"
		}
		t.Run(name, func(t *testing.T) {
			f := newRepresentationFixture(t)
			old := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "draft/item"}
			next := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/corrected", AspectName: "properties"}
			seed, err := f.writer.Commit("seed", f.change(testkit.MustHead(t, f.store, snapshot.DefaultRef), []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: old, PathHint: "public/item.yaml", Value: "original"},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "private/notes"}, PathHint: "private/notes.yaml", Value: "unselected secret"},
			}))
			if err != nil {
				t.Fatal(err)
			}
			operations := []objectmodel.Operation{
				{Op: objectmodel.OpRemove, Address: old},
				{Op: objectmodel.OpPut, Address: next, PathHint: "public/item.yaml", Value: "corrected"},
			}
			if putFirst {
				operations[0], operations[1] = operations[1], operations[0]
			}
			accepted, err := f.writer.Commit("explicit-replacement", f.change(seed.Result.NewCommit, operations))
			if err != nil {
				t.Fatal(err)
			}
			for _, at := range []struct {
				commit          kernel.CommitID
				present, absent objectmodel.Address
				value           string
			}{
				{seed.Result.NewCommit, old, next, "original"},
				{accepted.Result.NewCommit, next, old, "corrected"},
			} {
				fixed, err := f.reader.ReadAddress(f.store.ID(), at.present, at.commit)
				if err != nil || fixed.Value != at.value || fixed.Commit != at.commit {
					t.Fatalf("fixed identity not preserved at %s: %#v %v", at.commit, fixed, err)
				}
				if _, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: at.absent.ObjectID}, at.commit, nil); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					t.Fatalf("absent identity %s has a locator at %s: %v", at.absent.ObjectID, at.commit, err)
				}
				scope := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
					return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
				}, reader.DatasetPin{
					SetID: "selected-file-only", Repositories: map[kernel.RepositoryID]kernel.CommitID{f.store.ID(): at.commit},
					Items: []reader.DatasetItem{{Repository: f.store.ID(), Commit: at.commit, Kind: "file", File: "public/item.yaml", Target: "renamed-delivery.yaml"}},
				})
				ref := objectmodel.Reference{Repository: f.store.ID(), Object: at.present.ObjectID}
				if at.present.AspectName != "" {
					ref.Locator = "/" + at.present.AspectName
				}
				rows, err := scope.ReadReference(ref)
				if err != nil || len(rows) != 1 || rows[0].Value != at.value || rows[0].Commit != at.commit {
					t.Fatalf("selected-file Dataset changed fixed identity or value: %#v %v", rows, err)
				}
				ref.Object, ref.Locator = at.absent.ObjectID, ""
				if _, err := scope.ReadReference(ref); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					t.Fatalf("Dataset retained the absent identity: %v", err)
				}
				ref.Object = "private/notes"
				if _, err := scope.ReadReference(ref); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					t.Fatalf("selected-file Dataset read an unselected sibling: %v", err)
				}
				private, err := f.reader.Read(objectmodel.ObjectRef{Repository: f.store.ID(), Object: "private/notes"}, at.commit, nil)
				if err != nil || private.Value != "unselected secret" {
					t.Fatalf("explicit replacement changed a neighboring object: %#v %v", private, err)
				}
			}
		})
	}
}

func TestFinalMaintainedUnitsCannotShareAPath(t *testing.T) {
	first := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "definition"}, PathHint: "shared/item.yaml", Value: "first"}
	for _, next := range []objectmodel.Address{
		{Kind: objectmodel.KindAspect, ObjectID: "item/b", AspectName: "definition"},
		{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "usage"},
	} {
		for _, existing := range []bool{false, true} {
			f := newRepresentationFixture(t)
			before := testkit.MustHead(t, f.store, snapshot.DefaultRef)
			operations := []objectmodel.Operation{first}
			if existing {
				seed, err := f.writer.Commit("seed", f.change(before, operations))
				if err != nil {
					t.Fatal(err)
				}
				before, operations = seed.Result.NewCommit, nil
			}
			operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: next, PathHint: first.PathHint, Value: "second"})
			_, err := f.writer.Commit("path-collision", f.change(before, operations))
			if kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
				t.Fatalf("distinct maintained units shared a path: existing=%v next=%#v error=%v", existing, next, err)
			}
			if head := testkit.MustHead(t, f.store, snapshot.DefaultRef); head != before {
				t.Fatal("path collision published a partial batch")
			}
		}
	}
}

func TestPathReuseRequiresRemovalOfItsOwningMember(t *testing.T) {
	for _, removal := range []string{"whole_object", "exact_member", "sibling_member"} {
		for _, putFirst := range []bool{false, true} {
			order := "remove_then_put"
			if putFirst {
				order = "put_then_remove"
			}
			t.Run(removal+"/"+order, func(t *testing.T) {
				f := newRepresentationFixture(t)
				owner := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "old/item", AspectName: "parts", MemberKey: "selected"}
				sibling := owner
				sibling.MemberKey = "neighbor"
				next := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "new/item"}
				seed, err := f.writer.Commit("seed", f.change(testkit.MustHead(t, f.store, snapshot.DefaultRef), []objectmodel.Operation{
					{Op: objectmodel.OpPut, Address: owner, PathHint: "shared/item.yaml", Value: "selected member"},
					{Op: objectmodel.OpPut, Address: sibling, PathHint: "shared/neighbor.yaml", Value: "neighbor member"},
				}))
				if err != nil {
					t.Fatal(err)
				}
				remove := owner
				switch removal {
				case "whole_object":
					remove = objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: owner.ObjectID}
				case "sibling_member":
					remove = sibling
				}
				operations := []objectmodel.Operation{
					{Op: objectmodel.OpRemove, Address: remove},
					{Op: objectmodel.OpPut, Address: next, PathHint: "shared/item.yaml", Value: "replacement"},
				}
				if putFirst {
					operations[0], operations[1] = operations[1], operations[0]
				}
				receipt, err := f.writer.Commit("replace-member-path", f.change(seed.Result.NewCommit, operations))
				if removal == "sibling_member" {
					if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || testkit.MustHead(t, f.store, snapshot.DefaultRef) != seed.Result.NewCommit {
						t.Fatalf("removing a sibling released the occupied member path: %#v %v", receipt, err)
					}
					for _, address := range []objectmodel.Address{owner, sibling} {
						if _, err := f.reader.ReadAddress(f.store.ID(), address, seed.Result.NewCommit); err != nil {
							t.Fatalf("rejected replacement changed an original member: %v", err)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				current, err := f.reader.ReadAddress(f.store.ID(), next, receipt.Result.NewCommit)
				if err != nil || current.Value != "replacement" {
					t.Fatalf("explicit owner removal did not transfer the path: %#v %v", current, err)
				}
				if _, err := f.reader.ReadAddress(f.store.ID(), owner, receipt.Result.NewCommit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					t.Fatalf("removed member retained a stale locator: %v", err)
				}
				neighbor, err := f.reader.ReadAddress(f.store.ID(), sibling, receipt.Result.NewCommit)
				if removal == "exact_member" {
					if err != nil || neighbor.Value != "neighbor member" {
						t.Fatalf("exact member removal changed its neighbor: %#v %v", neighbor, err)
					}
				} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					t.Fatalf("whole-object removal retained another member locator: %v", err)
				}
				original, err := f.reader.ReadAddress(f.store.ID(), owner, seed.Result.NewCommit)
				if err != nil || original.Value != "selected member" {
					t.Fatalf("path transfer changed the original fixed commit: %#v %v", original, err)
				}
			})
		}
	}
}

func TestObjectPathDoesNotSilentlyReplaceAnUnselectedRawFile(t *testing.T) {
	f := newRepresentationFixture(t)
	registry := snapshot.NewRegistry()
	if err := registry.Add(f.store); err != nil {
		t.Fatal(err)
	}
	rawWriter, err := snapshot.NewRawWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, f.store, snapshot.DefaultRef)
	seed, err := rawWriter.Commit("raw-file", snapshot.TreeChangeSet{
		TargetRepository: f.store.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "raw/original.yaml", Content: []byte("original non-object bytes\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/new"}, PathHint: "raw/original.yaml", Value: "new object"}
	if _, err := f.writer.Commit("raw-file-collision", f.change(seed.Result.NewCommit, []objectmodel.Operation{operation})); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("Object publication silently overwrote an unselected Snapshot file: %v", err)
	}
	if head := testkit.MustHead(t, f.store, snapshot.DefaultRef); head != seed.Result.NewCommit {
		t.Fatal("raw path collision advanced HEAD")
	}
	// Recovery leaves the pure file alone and chooses an unused object path.
	operation.PathHint = "objects/new.yaml"
	recovered, err := f.writer.Commit("choose-free-path", f.change(seed.Result.NewCommit, []objectmodel.Operation{operation}))
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := snapshot.TreeStoreOf(f.store)
	original, err := tree.ReadFile("raw/original.yaml", recovered.Result.NewCommit)
	if err != nil || string(original) != "original non-object bytes\n" {
		t.Fatalf("path recovery changed the original pure file: %q %v", original, err)
	}
}

func TestLakeFSWriterPathOccupancyKeepsMissingAndSameIdentityWrites(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	raw, err := snapshot.OpenLakeFS("kr://path-identity/lakefs", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := raw.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	repo := testkit.OpenRepository(t, raw)
	before := testkit.MustHead(t, raw, snapshot.DefaultRef)
	tree, _ := snapshot.TreeStoreOf(raw)
	if _, err := tree.ReadFile("objects/item.yaml", before); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("lakeFS missing destination must match the ordinary fixture contract: %v", err)
	}
	operation := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/existing"}, PathHint: "objects/item.yaml", Value: "created"}
	for _, value := range []string{"created", "updated"} {
		operation.Value = value
		before, err = repo.ApplyObjectCommit(objectmodel.ChangeSet{
			TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before,
			Operations: []objectmodel.Operation{operation},
		})
		if err != nil {
			t.Fatalf("lakeFS new destination or same-identity update rejected: %v", err)
		}
	}
	operation.Address.ObjectID = "item/different"
	_, err = repo.ApplyObjectCommit(objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before,
		Operations: []objectmodel.Operation{operation},
	})
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || testkit.MustHead(t, raw, snapshot.DefaultRef) != before {
		t.Fatalf("lakeFS path takeover did not preserve the accepted commit: %v", err)
	}
	value, err := repo.Read("item/existing", before)
	if err != nil || value.Value != "updated" {
		t.Fatalf("legal same-identity update lost its content: %#v %v", value, err)
	}
}
