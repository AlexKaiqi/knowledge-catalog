package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

func TestNativeSchemaBatchMigrationAndRootRemovalUseFinalUnits(t *testing.T) {
	base := testkit.MakeRepository(t, "kr://writer/native-schema-final")
	native := &nativeSchemaRepository{KnowledgeRepository: base}
	registry := snapshot.NewRegistry()
	if err := registry.Add(native); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, native, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "definition"}
	seed, err := w.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: native.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}, Value: map[string]any{"entity": "Policy", "aspect": "definition"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}, Value: map[string]any{"entity": "Policy", "aspect": "definition"}},
			{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/a", Value: map[string]any{"name": "A"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "policy/A", AspectName: "tags", MemberKey: "kept"}, Value: "tag"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("new-referrer-and-removal", objectmodel.ChangeSet{
		TargetRepository: native.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: seed.Result.NewCommit, ExpectedTargetCommit: seed.Result.NewCommit,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/new", AspectName: "definition"}, SchemaRef: "schema/b", Value: map[string]any{"name": "new"}},
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if head := testkit.MustHead(t, native, snapshot.DefaultRef); head != seed.Result.NewCommit {
		t.Fatalf("native Schema removal left new dangling referrer at %s", head)
	}
	migrated, err := w.Commit("migrate", objectmodel.ChangeSet{
		TargetRepository: native.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: seed.Result.NewCommit, ExpectedTargetCommit: seed.Result.NewCommit,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/b", Value: map[string]any{"name": "migrated"}},
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}},
		},
	})
	if err != nil {
		t.Fatalf("native batch migration must discharge old Schema referrer: %v", err)
	}
	_, err = w.Commit("remove-root-and-schema", objectmodel.ChangeSet{
		TargetRepository: native.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: migrated.Result.NewCommit, ExpectedTargetCommit: migrated.Result.NewCommit,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"}},
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}},
		},
	})
	if err != nil {
		t.Fatalf("native root removal must discharge all deleted unit refs: %v", err)
	}
	if _, err := native.ReadAddress(address, seed.Result.NewCommit); err != nil {
		t.Fatalf("native old referenced unit lost its fixed version: %v", err)
	}
}
