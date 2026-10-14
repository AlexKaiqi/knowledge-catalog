package writer_test

import (
	"fmt"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type guardedSchemaTree struct {
	snapshot.Store
	snapshot.TreeStore
	guarded bool
	poison  map[string]bool
}

func (r *guardedSchemaTree) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if r.guarded && r.poison[path] {
		return nil, fmt.Errorf("unrelated unit %s must not be read", path)
	}
	return r.TreeStore.ReadFile(path, commit)
}

func (r *guardedSchemaTree) ListFiles(commit kernel.CommitID) ([]string, error) {
	if r.guarded {
		return nil, fmt.Errorf("Schema maintenance must not scan tree at %s", commit)
	}
	return r.TreeStore.ListFiles(commit)
}

func newGuardedSchemaWriter(t *testing.T) (*guardedSchemaTree, *writer.Writer, *reader.Reader) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://writer/bounded-schema")
	guard := &guardedSchemaTree{Store: raw, TreeStore: raw.(snapshot.TreeStore), poison: map[string]bool{}}
	registry := snapshot.NewRegistry()
	if err := registry.Add(guard); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	return guard, w, reader.NewReader(registry)
}

func schemaTreeCommit(t *testing.T, tree *guardedSchemaTree, w *writer.Writer, command string, base kernel.CommitID, operations []objectmodel.Operation) kernel.CommitID {
	t.Helper()
	receipt, err := w.Commit(command, objectmodel.ChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: operations,
	})
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return receipt.Result.NewCommit
}

func assertTreeSchemaReferrers(t *testing.T, r *reader.Reader, repo kernel.RepositoryID, schema objectmodel.ObjectID, commit kernel.CommitID, want []objectmodel.Address) {
	t.Helper()
	view, err := r.Require(repo, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	got, err := view.(objectmodel.SchemaReferrerLocator).SchemaReferrerAddresses(schema, commit)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("fixed reverse Schema coordinates at %s: got=%#v want=%#v error=%v", commit, got, want, err)
	}
}

func TestTreeSchemaEvolutionUsesOnlyReferencedUnits(t *testing.T) {
	tree, w, r := newGuardedSchemaWriter(t)
	root := testkit.MustHead(t, tree, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "definition"}
	schema := func(typedTier bool) map[string]any {
		fields := map[string]any{"extra": map[string]any{"type": "string"}}
		if typedTier {
			fields["tier"] = map[string]any{"type": "integer"}
		}
		return map[string]any{"entity": "Policy", "aspect": "definition", "additionalProperties": true, "fields": fields}
	}
	first := schemaTreeCommit(t, tree, w, "seed", root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: map[string]any{"entity": "Policy", "aspect": "definition", "additionalProperties": true}},
		{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/policy", PathHint: "policy/definition.yaml", Value: map[string]any{"tier": "gold"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "description"}, PathHint: "policy/description.yaml", Value: "unrelated sibling"},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/unrelated"}, PathHint: "notes/unrelated.yaml", Value: "unrelated object"},
	})
	tree.guarded = true
	tree.poison["policy/description.yaml"], tree.poison["notes/unrelated.yaml"] = true, true
	compatible := schemaTreeCommit(t, tree, w, "optional-field", first, testkit.PutEntity("schema/policy", schema(false), ""))
	_, err := w.Commit("invalid-instance", objectmodel.ChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: compatible, ExpectedTargetCommit: compatible,
		Operations: testkit.PutEntity("schema/policy", schema(true), ""),
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if head := testkit.MustHead(t, tree, snapshot.DefaultRef); head != compatible {
		t.Fatalf("unsafe bounded Schema change moved Ref to %s", head)
	}
	// Replacing this object's unit requires its sibling representation; the
	// other object remains inaccessible throughout the successful migration.
	delete(tree.poison, "policy/description.yaml")
	migrated := schemaTreeCommit(t, tree, w, "migrate-instance", compatible, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: schema(true)},
		{Op: objectmodel.OpPut, Address: address, PathHint: "moved/definition.yaml", Value: map[string]any{"tier": 1}},
	})
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/policy", migrated, []objectmodel.Address{address})
	value, err := r.ReadAddress(tree.ID(), address, migrated)
	if err != nil || kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(map[string]any{"tier": 1}) || value.Declarations[0].SchemaRef != "schema/policy" {
		t.Fatalf("same-batch inherited Schema migration was lost: %#v, %v", value, err)
	}
}

func TestTreeSchemaReferrersFollowBatchMigrationAndRootRemoval(t *testing.T) {
	tree, w, r := newGuardedSchemaWriter(t)
	root := testkit.MustHead(t, tree, snapshot.DefaultRef)
	a := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "definition"}
	b := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/B", AspectName: "definition"}
	schema := map[string]any{"entity": "Policy", "aspect": "definition"}
	first := schemaTreeCommit(t, tree, w, "seed", root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}, Value: schema},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}, Value: map[string]any{"entity": "Policy", "aspect": "definition"}},
		{Op: objectmodel.OpPut, Address: a, SchemaRef: "schema/a", Value: map[string]any{"name": "A"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "policy/A", AspectName: "tags", MemberKey: "keep"}, Value: "sibling"},
	})
	tree.guarded = true
	inherited := schemaTreeCommit(t, tree, w, "inherit", first, []objectmodel.Operation{{Op: objectmodel.OpPut, Address: a, Value: map[string]any{"name": "updated"}}})
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/a", inherited, []objectmodel.Address{a})
	migrated := schemaTreeCommit(t, tree, w, "migrate-and-add", inherited, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: a, SchemaRef: "schema/b", Value: map[string]any{"name": "migrated"}},
		{Op: objectmodel.OpPut, Address: b, SchemaRef: "schema/b", Value: map[string]any{"name": "new"}},
		{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/a"}},
	})
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/a", migrated, nil)
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/b", migrated, []objectmodel.Address{a, b})
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/a", first, []objectmodel.Address{a})
	removed := schemaTreeCommit(t, tree, w, "drop-referrers-and-schema", migrated, []objectmodel.Operation{
		{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"}},
		{Op: objectmodel.OpRemove, Address: b},
		{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/b"}},
	})
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/b", removed, nil)
	if _, err := r.ReadAddress(tree.ID(), a, removed); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("root removal left referenced Aspect: %v", err)
	}
	if _, err := r.ReadAddress(tree.ID(), a, first); err != nil {
		t.Fatalf("fixed old referrer is no longer readable: %v", err)
	}
}

func TestTreeSchemaPinnedLookupAvoidsCurrentAndUnrelatedBodies(t *testing.T) {
	tree, w, _ := newGuardedSchemaWriter(t)
	root := testkit.MustHead(t, tree, snapshot.DefaultRef)
	first := schemaTreeCommit(t, tree, w, "schema-first", root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: map[string]any{"entity": "Policy", "aspect": "definition"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/unrelated"}, PathHint: "notes/unrelated.yaml", Value: "unrelated"},
	})
	tree.guarded, tree.poison["notes/unrelated.yaml"] = true, true
	second := schemaTreeCommit(t, tree, w, "schema-new-constraint", first, testkit.PutEntity("schema/policy", map[string]any{
		"entity": "Policy", "aspect": "definition", "fields": map[string]any{"extra": map[string]any{"type": "integer"}},
	}, ""))
	schemaTreeCommit(t, tree, w, "instance-pinned-old", second, []objectmodel.Operation{{
		Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "definition"},
		SchemaRef: "schema/policy@" + string(first), Value: map[string]any{"extra": "valid only at old Schema"},
	}})
}

func TestTreeSchemaReferrerMetadataRequiresExplicitRebuild(t *testing.T) {
	tree, w, r := newGuardedSchemaWriter(t)
	root := testkit.MustHead(t, tree, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "definition"}
	first := schemaTreeCommit(t, tree, w, "seed", root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: map[string]any{"entity": "Policy", "aspect": "definition"}},
		{Op: objectmodel.OpPut, Address: address, SchemaRef: "schema/policy", Value: map[string]any{"tier": "gold"}},
	})
	legacy, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: first, ExpectedTargetCommit: first,
		Changes: []snapshot.TreeChange{
			{Path: repofile.LocatorReferrersCompletePath, Remove: true},
			{Path: repofile.SchemaReferrersPath("schema/policy"), Remove: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree.guarded = true
	_, err = w.Commit("missing-proof", objectmodel.ChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: legacy, ExpectedTargetCommit: legacy,
		Operations: testkit.PutEntity("schema/policy", map[string]any{
			"entity": "Policy", "aspect": "definition", "fields": map[string]any{"tier": map[string]any{"type": "integer"}},
		}, ""),
	})
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	if head := testkit.MustHead(t, tree, snapshot.DefaultRef); head != legacy {
		t.Fatalf("unproved Schema change advanced Ref to %s", head)
	}
	tree.guarded = false // Only the explicitly requested maintenance scan may enumerate.
	rebuilt, err := w.RebuildTreeLocators("rebuild-proof", tree.ID(), snapshot.DefaultRef, legacy)
	if err != nil {
		t.Fatal(err)
	}
	tree.guarded = true
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/policy", rebuilt.NewCommit, []objectmodel.Address{address})
	if _, err := repofile.ReadSchemaReferrers(tree, "schema/policy", legacy); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("rebuild retroactively supplied old-version metadata: %v", err)
	}
	assertTreeSchemaReferrers(t, r, tree.ID(), "schema/policy", first, []objectmodel.Address{address})
	_, err = w.Commit("proved-invalid", objectmodel.ChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: rebuilt.NewCommit, ExpectedTargetCommit: rebuilt.NewCommit,
		Operations: testkit.PutEntity("schema/policy", map[string]any{
			"entity": "Policy", "aspect": "definition", "fields": map[string]any{"tier": map[string]any{"type": "integer"}},
		}, ""),
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
}

func TestTreeSchemaRemovalRejectsNewReferrerInSameBatch(t *testing.T) {
	tree, w, _ := newGuardedSchemaWriter(t)
	root := testkit.MustHead(t, tree, snapshot.DefaultRef)
	first := schemaTreeCommit(t, tree, w, "schema", root, testkit.PutEntity("schema/policy", map[string]any{"entity": "Policy", "aspect": "definition"}, ""))
	tree.guarded = true
	_, err := w.Commit("new-referrer-and-removal", objectmodel.ChangeSet{
		TargetRepository: tree.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: first, ExpectedTargetCommit: first,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/new", AspectName: "definition"}, SchemaRef: "schema/policy", Value: map[string]any{"name": "new"}},
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if head := testkit.MustHead(t, tree, snapshot.DefaultRef); head != first {
		t.Fatalf("Schema removal left a new dangling referrer at %s", head)
	}
}
