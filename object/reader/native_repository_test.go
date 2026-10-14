package reader_test

import (
	"fmt"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type nativeReadFixture struct{ *testkit.ObjectRepository }

func (*nativeReadFixture) NativeObjectRepository() {}

func TestNativeReadViewPreservesBoundedObjectPorts(t *testing.T) {
	raw := testkit.MakeRepository(t, "kr://reader/native-ports")
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	const schemaID objectmodel.ObjectID = "schema/note"
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/one"}
	at, err := raw.ApplyObjectCommit(objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schemaID}, Value: map[string]any{
				"entity": "Note", "pattern": "record", "fields": map[string]any{"body": map[string]any{"type": "string"}},
			}},
			{Op: objectmodel.OpPut, Address: address, SchemaRef: string(schemaID), PathHint: "notes/one.yaml", Value: map[string]any{"body": "fixed native value"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := reader.NewReader(nil).Wrap(&nativeReadFixture{raw}, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := view.(objectmodel.UnitLocator).ObjectUnitPaths(address.ObjectID, at)
	if err != nil || !reflect.DeepEqual(paths, []string{"notes/one.yaml"}) {
		t.Fatalf("native unit locator lost fixed paths: %v %v", paths, err)
	}
	ids, err := view.(objectmodel.SchemaStore).SchemaObjectIDs(at)
	if err != nil || !reflect.DeepEqual(ids, []objectmodel.ObjectID{schemaID}) {
		t.Fatalf("native schema locator changed: %v %v", ids, err)
	}
	referrers, err := view.(objectmodel.SchemaReferrerLocator).SchemaReferrerAddresses(schemaID, at)
	if err != nil || !reflect.DeepEqual(referrers, []objectmodel.Address{address}) {
		t.Fatalf("native referrer locator changed: %#v %v", referrers, err)
	}
	values, err := view.(objectmodel.BatchReadStore).ReadMany([]objectmodel.ObjectID{address.ObjectID}, at)
	if err != nil || len(values) != 1 {
		t.Fatalf("native batch read lost its values: %#v %v", values, err)
	}
	got := values[address.ObjectID]
	if got.Repository != raw.ID() || got.Commit != at || got.ObjectRef.Object != address.ObjectID ||
		!reflect.DeepEqual(got.Value, map[string]any{"body": "fixed native value"}) {
		t.Fatalf("native batch read changed fixed coordinates or value: %#v", got)
	}
}

type unsafeNativeBindingFixture struct {
	*testkit.ObjectRepository
	bindingCalls int
}

func (*unsafeNativeBindingFixture) NativeObjectRepository() {}

func (r *unsafeNativeBindingFixture) BindingSchemaObjectIDs(kernel.CommitID) ([]objectmodel.ObjectID, error) {
	r.bindingCalls++
	return nil, fmt.Errorf("native binding classification would read unpublished bodies")
}

func TestDatasetNativeBindingCoordinatesRequireMetadataOnlyPort(t *testing.T) {
	s := testkit.NewSetup(t, "kr://reader/native-binding-scope")
	published, err := s.Writer.Commit("publish-public-schema", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
			Address:  objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/public"},
			PathHint: "public/schema.yaml", Value: map[string]any{"entity": "Note", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := &unsafeNativeBindingFixture{ObjectRepository: s.Repo}
	rd := reader.NewReader(nil)
	at := published.Result.NewCommit
	scope := reader.Open(rd.Lookup(func(kernel.RepositoryID) (snapshot.Store, error) { return raw, nil }), reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: at},
		Items:        []reader.DatasetItem{{Repository: s.RepositoryID, Commit: at, Kind: "prefix", Prefix: "public"}},
	})
	member, err := scope.Member(s.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := member.(objectmodel.SchemaStore).SchemaObjectIDs(at)
	if err != nil || !reflect.DeepEqual(ids, []objectmodel.ObjectID{"schema/public"}) {
		t.Fatalf("positive scoped Schema namespace was not established: %v %v", ids, err)
	}
	if _, err := member.(objectmodel.BindingLocator).BindingSchemaObjectIDs(at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unsafe native binding lookup must fail explicitly: %v", err)
	}
	if raw.bindingCalls != 0 {
		t.Fatalf("scope trusted an arbitrary native binding classifier: %d", raw.bindingCalls)
	}
}

func TestNativeReadViewRejectsMissingBoundedPort(t *testing.T) {
	raw := testkit.MakeNativeSystemRepository(t)
	view, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	at := testkit.MustHead(t, raw, snapshot.DefaultRef)
	if _, err := view.(objectmodel.UnitLocator).ObjectUnitPaths(objectmodel.SystemReadmeObjectID, at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("native provider without unit locator must fail explicitly: %v", err)
	}
}

type nativeReferenceCounter struct {
	testkit.NativeSnapshotRepository
	reads int
}

func (r *nativeReferenceCounter) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	r.reads++
	return r.NativeSnapshotRepository.Read(id, at)
}

func TestNativeReferenceSelectionWithoutPlanningPortDoesNotLoadObject(t *testing.T) {
	raw := &nativeReferenceCounter{NativeSnapshotRepository: testkit.MakeNativeSystemRepository(t)}
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	at := testkit.MustHead(t, raw, snapshot.DefaultRef)
	ref := objectmodel.Reference{Repository: raw.ID(), Object: objectmodel.SystemReadmeObjectID}
	value, err := rd.ReadReference(ref, at)
	if err != nil || value.Commit != at || value.Digest != kernel.CanonicalDigest(value.Value) || raw.reads != 1 {
		t.Fatalf("native root read changed fixed object: %#v reads=%d err=%v", value, raw.reads, err)
	}
	ref.Locator = "/readme"
	if _, err := rd.ReadReference(ref, at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("native selection without a planning port must fail explicitly: %v", err)
	}
	if raw.reads != 1 {
		t.Fatalf("native selection loaded the whole object before declaring a missing capability: %d", raw.reads)
	}
	if _, err := rd.ReadReferenceUnits(ref, at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("native complete-unit selection without its port must fail explicitly: %v", err)
	}
	if raw.reads != 1 {
		t.Fatalf("native complete-unit selection loaded the object: %d", raw.reads)
	}
}
