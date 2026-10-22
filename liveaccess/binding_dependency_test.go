package liveaccess_test

import (
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

// accept-cross-descriptor-repo-not-selected: a Dataset selects table-meta but
// not ops-shared, whose ops/warehouse-stats the live-stats Aspect names.
// Publication freezes ops-shared as a dependency version; the read resolves
// there, and ops-shared stays out of every consumer-visible list.
func TestDescriptorInUnselectedRepositoryResolvesAtFrozenDependency(t *testing.T) {
	f := newDescriptorDependencyFixture(t)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/shop.orders", AspectName: "live-stats"}

	selectedOnly := reader.Open(f.lookup, f.pin(nil))
	if _, err := liveaccess.ResolveBindingAt(selectedOnly, f.table, address); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("without a frozen dependency the unselected descriptor must not resolve, got %v", err)
	}
	dependencies, err := selectedOnly.FreezeDependencies(f.registry.IDs)
	wantDependencies := map[kernel.RepositoryID]kernel.CommitID{f.ops: f.opsAt, f.table: f.tableAt}
	if err != nil || !reflect.DeepEqual(dependencies, wantDependencies) {
		t.Fatalf("publication did not freeze the holder and its absence witnesses: %v %v", dependencies, err)
	}

	scope := reader.Open(f.lookup, f.pin(dependencies))
	resolved, err := liveaccess.ResolveBindingAt(scope, f.table, address)
	if err != nil || resolved.DescriptorRepository != f.ops || resolved.DescriptorCommit != f.opsAt || resolved.DescriptorDigest == "" || resolved.Origin != "https://live-origin.example" {
		t.Fatalf("live-stats did not resolve at the frozen dependency version: %#v %v", resolved, err)
	}

	// The dependency repository moving on does not move the revision.
	moved, err := f.writer.Commit("advance-descriptor", objectmodel.ChangeSet{
		TargetRepository: f.ops, TargetRef: snapshot.DefaultRef, BaseCommit: f.opsAt, ExpectedTargetCommit: f.opsAt,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("ops/warehouse-stats", bindingfixture.Descriptor("https://moved.example"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := liveaccess.ResolveBindingAt(scope, f.table, address)
	if err != nil || again.DescriptorCommit != f.opsAt || again.Origin != resolved.Origin || again.DeclarationDigest != resolved.DeclarationDigest {
		t.Fatalf("frozen dependency followed the dependency head %s: %#v %v", moved.Result.NewCommit, again, err)
	}

	// Interpretation only: not a member, not readable, not held.
	if values, err := scope.Read("ops/warehouse-stats"); err != nil || len(values) != 0 {
		t.Fatalf("dependency descriptor leaked into the Dataset object read: %#v %v", values, err)
	}
	if held, err := scope.Holds("ops/warehouse-stats"); err != nil || held {
		t.Fatalf("dependency descriptor counted as held by the Dataset: %v %v", held, err)
	}
	if inScope, err := scope.Contains(f.ops, "ops/warehouse-stats"); err != nil || inScope {
		t.Fatalf("dependency repository counted as in scope: %v %v", inScope, err)
	}
	if _, err := scope.Member(f.ops); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("dependency repository opened as a Dataset member: %v", err)
	}
	if _, listed := scope.Pin().Repositories[f.ops]; listed {
		t.Fatal("dependency repository joined the pinned members")
	}
}

func TestDescriptorDependencyFreezeFailsWithoutSingleHolder(t *testing.T) {
	f := newDescriptorDependencyFixture(t)
	selectedOnly := reader.Open(f.lookup, f.pin(nil))
	onlyTable := func() []kernel.RepositoryID { return []kernel.RepositoryID{f.table} }
	if _, err := selectedOnly.FreezeDependencies(onlyTable); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("a descriptor with no holder must fail preparation, got %v", err)
	}

	second := testkit.MakeTreeStore(t, "kr://dependency/ops-copy")
	if err := f.registry.Add(second); err != nil {
		t.Fatal(err)
	}
	base := testkit.MustHead(t, second, snapshot.DefaultRef)
	if _, err := f.writer.Commit("copy-descriptor", objectmodel.ChangeSet{
		TargetRepository: second.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("ops/warehouse-stats", bindingfixture.Descriptor("https://copy.example"))},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedOnly.FreezeDependencies(f.registry.IDs); kernel.CodeOf(err) != kernel.ErrObjectIDConflict {
		t.Fatalf("a descriptor held by two unselected repositories must fail preparation, got %v", err)
	}
}

type descriptorDependencyFixture struct {
	registry snapshot.Registry
	writer   *writer.Writer
	lookup   reader.MemberLookup
	table    kernel.RepositoryID
	tableAt  kernel.CommitID
	ops      kernel.RepositoryID
	opsAt    kernel.CommitID
}

func newDescriptorDependencyFixture(t *testing.T) *descriptorDependencyFixture {
	t.Helper()
	ops := testkit.MakeTreeStore(t, "kr://dependency/ops-shared")
	table := testkit.MakeTreeStore(t, "kr://dependency/table-meta")
	registry := snapshot.NewRegistry()
	for _, store := range []snapshot.Store{ops, table} {
		if err := registry.Add(store); err != nil {
			t.Fatal(err)
		}
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	opsBase := testkit.MustHead(t, ops, snapshot.DefaultRef)
	opsCommit, err := w.Commit("publish-descriptor", objectmodel.ChangeSet{
		TargetRepository: ops.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: opsBase, ExpectedTargetCommit: opsBase,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("ops/warehouse-stats", bindingfixture.Descriptor("https://live-origin.example"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	tableBase := testkit.MustHead(t, table, snapshot.DefaultRef)
	tableCommit, err := w.Commit("publish-live-stats", objectmodel.ChangeSet{
		TargetRepository: table.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: tableBase, ExpectedTargetCommit: tableBase,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.live-stats"}, Value: map[string]any{
				"entity": "Table", "aspect": "live-stats", "fields": map[string]any{"rowCount": map[string]any{"type": "integer"}},
			}},
			bindingfixture.PutAspect("table/shop.orders", "live-stats", "schema/table.live-stats", "ops/warehouse-stats"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	objects := reader.NewReader(registry)
	return &descriptorDependencyFixture{
		registry: registry, writer: w,
		lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return objects.Require(id, kernel.ErrObjectRefUnresolved)
		},
		table: table.ID(), tableAt: tableCommit.Result.NewCommit,
		ops: ops.ID(), opsAt: opsCommit.Result.NewCommit,
	}
}

func (f *descriptorDependencyFixture) pin(dependencies map[kernel.RepositoryID]kernel.CommitID) reader.DatasetPin {
	members := map[kernel.RepositoryID]kernel.CommitID{f.table: f.tableAt}
	return reader.DatasetPin{SetID: "sales-analysis", Revision: 1, Repositories: members, Items: reader.WholeRepositoryItems(members), Dependencies: dependencies}
}
