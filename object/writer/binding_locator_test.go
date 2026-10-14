package writer_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/writer"
	"kc/snapshot"
)

func TestTreeWriterMaintainsBindingSchemaCoordinates(t *testing.T) {
	raw, tree, w := newBindingCoordinateWriter(t)
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	schema := func(origin string) map[string]any {
		value := map[string]any{"entity": "Table", "aspect": "stats", "fields": map[string]any{"rows": map[string]any{"type": "number"}}}
		if origin != "" {
			value["origin"] = origin
		}
		return value
	}
	first, err := w.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}, Value: schema("https://a.example")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}, Value: schema("https://b.example")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/static"}, Value: schema("")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, first.Result.NewCommit, []objectmodel.ObjectID{"schema/a", "schema/b"})
	second, err := w.Commit("change-directory", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.NewCommit, ExpectedTargetCommit: first.Result.NewCommit,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}, Value: schema("")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/static"}, Value: schema("https://static.example")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, second.Result.NewCommit, []objectmodel.ObjectID{"schema/b", "schema/static"})
	third, err := w.Commit("remove-schema", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: second.Result.NewCommit, ExpectedTargetCommit: second.Result.NewCommit,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, third.Result.NewCommit, []objectmodel.ObjectID{"schema/static"})
	assertBindingCoordinates(t, tree, first.Result.NewCommit, []objectmodel.ObjectID{"schema/a", "schema/b"})
	if _, err := tree.ReadFile(repofile.LocatorManifestPath, third.Result.NewCommit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("incremental Schema write introduced a whole-repository manifest: %v", err)
	}
	rebuilt, err := w.RebuildTreeLocators("rebuild-bindings", raw.ID(), snapshot.DefaultRef, third.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, rebuilt.NewCommit, []objectmodel.ObjectID{"schema/static"})
}

func newBindingCoordinateWriter(t *testing.T) (snapshot.Store, snapshot.TreeStore, *writer.Writer) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://writer/binding-coordinates")
	tree := raw.(snapshot.TreeStore)
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw, tree, w
}

func TestTreeWriterRequiresExplicitBindingCoordinateRebuild(t *testing.T) {
	raw, tree, w := newBindingCoordinateWriter(t)
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	bound := map[string]any{"entity": "Table", "aspect": "stats", "origin": "https://stats.example"}
	first, err := w.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("schema/stats", bound, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.NewCommit, ExpectedTargetCommit: first.Result.NewCommit,
		Changes: []snapshot.TreeChange{{Path: repofile.LocatorBindingIndexPath, Remove: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("blocked", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: legacy, ExpectedTargetCommit: legacy,
		Operations: testkit.PutEntity("schema/new", map[string]any{"entity": "Table", "aspect": "description"}, ""),
	})
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	if head := testkit.MustHead(t, raw, snapshot.DefaultRef); head != legacy {
		t.Fatalf("missing Binding coordinates moved Ref to %s", head)
	}
	rebuilt, err := w.RebuildTreeLocators("rebuild", raw.ID(), snapshot.DefaultRef, legacy)
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, rebuilt.NewCommit, []objectmodel.ObjectID{"schema/stats"})
	updated, err := w.Commit("static-schema", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: rebuilt.NewCommit, ExpectedTargetCommit: rebuilt.NewCommit,
		Operations: testkit.PutEntity("schema/stats", map[string]any{"entity": "Table", "aspect": "stats"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, updated.Result.NewCommit, nil)
}

func assertBindingCoordinates(t *testing.T, tree snapshot.TreeReader, commit kernel.CommitID, want []objectmodel.ObjectID) {
	t.Helper()
	raw, err := tree.ReadFile(repofile.LocatorBindingIndexPath, commit)
	if err != nil {
		t.Fatalf("fixed Binding coordinates are missing: %v", err)
	}
	ids, err := repofile.DecodeSchemaIndex(raw)
	if err != nil || !reflect.DeepEqual(ids, want) {
		t.Fatalf("Binding coordinates at %s: got=%v want=%v error=%v", commit, ids, want, err)
	}
}
