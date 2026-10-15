package writer_test

import (
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
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
	schema := map[string]any{"entity": "Table", "aspect": "stats", "fields": map[string]any{"rows": map[string]any{"type": "number"}}}
	snapshotUnit := func(object objectmodel.ObjectID, schemaRef string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: object, AspectName: "stats"}, SchemaRef: schemaRef, Value: map[string]any{"rows": 1}}
	}
	first, err := w.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}, Value: schema},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}, Value: schema},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/static"}, Value: schema},
			bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example")),
			bindingfixture.PutAspect("table/x", "stats", "schema/a", "resource/stats"),
			bindingfixture.PutAspect("table/y", "stats", "schema/b", "resource/stats"),
			snapshotUnit("table/z", "schema/static"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, first.Result.NewCommit, []objectmodel.ObjectID{"schema/a", "schema/b"})
	second, err := w.Commit("change-sources", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.NewCommit, ExpectedTargetCommit: first.Result.NewCommit,
		Operations: []objectmodel.Operation{
			snapshotUnit("table/x", "schema/a"),
			bindingfixture.PutAspect("table/z", "stats", "schema/static", "resource/stats"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, second.Result.NewCommit, []objectmodel.ObjectID{"schema/b", "schema/static"})
	third, err := w.Commit("remove-binding-unit", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: second.Result.NewCommit, ExpectedTargetCommit: second.Result.NewCommit,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/y", AspectName: "stats"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBindingCoordinates(t, tree, third.Result.NewCommit, []objectmodel.ObjectID{"schema/static"})
	assertBindingCoordinates(t, tree, first.Result.NewCommit, []objectmodel.ObjectID{"schema/a", "schema/b"})
	if _, err := tree.ReadFile(repofile.LocatorManifestPath, third.Result.NewCommit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("incremental write introduced a whole-repository manifest: %v", err)
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
	first, err := w.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/stats"}, Value: map[string]any{"entity": "Table", "aspect": "stats"}},
			bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example")),
			bindingfixture.PutAspect("table/orders", "stats", "schema/stats", "resource/stats"),
		},
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
	updated, err := w.Commit("snapshot-unit", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: rebuilt.NewCommit, ExpectedTargetCommit: rebuilt.NewCommit,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}, SchemaRef: "schema/stats", Value: map[string]any{}}},
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
	coordinates, err := repofile.DecodeBindingIndex(raw)
	ids := repofile.BindingSchemas(coordinates)
	if len(want) == 0 && len(ids) == 0 {
		return
	}
	if err != nil || !reflect.DeepEqual(ids, want) {
		t.Fatalf("Binding coordinates at %s: got=%v want=%v error=%v", commit, ids, want, err)
	}
}
