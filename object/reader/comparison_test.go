package reader_test

import (
	"context"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestComparisonReadsFullSelectedUnitWithoutHeadsSchemaOrUnselectedFiles(t *testing.T) {
	_, guard, w, at := newReferenceReadFixture(t)
	schema, err := w.Commit("comparison-schema", objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/column"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Column", "aspect": "columns", "pattern": "keyed_collection", "fields": map[string]any{"type": map[string]any{"type": "string"}, "owner": map[string]any{"type": "string"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	put := func(command string, base kernel.CommitID, owner string) kernel.CommitID {
		t.Helper()
		receipt, err := w.Commit(command, objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"}, SchemaRef: "schema/column", Value: map[string]any{"type": "string", "owner": owner}}}})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.Result.NewCommit
	}
	before := put("comparison-before", schema.Result.NewCommit, "trade")
	after := put("comparison-after", before, "ops")
	wrapped := &basisSnapshot{referenceReadGuard: guard, binding: "comparison-authority"}
	repo, err := reader.NewReader(nil).Wrap(wrapped, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			t.Fatalf("local comparison opened interpretation dependency %s", id)
		}
		return repo, nil
	}
	makeBasis := func(commit kernel.CommitID) objectmodel.ReadBasis {
		scope := reader.Open(lookup, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "file", File: "public/id.yaml"}}})
		basis, err := scope.ReadBasis(objectmodel.Reference{Object: "table/orders", Locator: "/columns/order_id/type"}, "")
		if err != nil {
			t.Fatal(err)
		}
		basis.Dependencies = []objectmodel.ReadSource{{Repository: "kr://comparison/unavailable-schema-source", Commit: "missing", Binding: "missing-authority"}}
		basis.UnresolvedDependencies = []objectmodel.ReadDependencyFailure{{Repository: "kr://comparison/unknown-source", Code: kernel.ErrObjectRefUnresolved}}
		return basis
	}
	leftBasis, rightBasis := makeBasis(before), makeBasis(after)
	guard.reads = map[string]int{}
	for _, path := range []string{"private/description.yaml", "private/status.yaml", "private/schema.yaml"} {
		guard.blocked[path] = true
	}
	left, err := reader.ReadComparisonBasis(context.Background(), lookup, leftBasis)
	if err != nil {
		t.Fatal(err)
	}
	right, err := reader.ReadComparisonBasis(context.Background(), lookup, rightBasis)
	if err != nil {
		t.Fatal(err)
	}
	result, err := objectmodel.CompareLocalReads(left, right)
	if err != nil || result.Equal == nil || *result.Equal || len(result.Changes) != 1 || !result.Changes[0].ValueChanged {
		t.Fatalf("field projection hid complete-unit change: %#v %v", result, err)
	}
	if left.Units[0].Value.(map[string]any)["type"] != "string" || left.Units[0].Value.(map[string]any)["owner"] != "trade" {
		t.Fatal("comparison trimmed complete unit to requested leaf")
	}
	for path, blocked := range guard.blocked {
		if blocked && guard.reads[path] != 0 {
			t.Fatalf("comparison read unselected or interpretation body %s", path)
		}
	}
}

func TestComparisonRootSliceRetainsOnlyPublishedUnitsAndAbsentCandidates(t *testing.T) {
	_, guard, _, at := newReferenceReadFixture(t)
	wrapped := &basisSnapshot{referenceReadGuard: guard, binding: "comparison-slice"}
	repo, err := reader.NewReader(nil).Wrap(wrapped, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }
	scope := reader.Open(lookup, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}}})
	basis, err := scope.ReadBasis(objectmodel.Reference{Object: "table/orders"}, "")
	if err != nil {
		t.Fatal(err)
	}
	guard.blocked["private/description.yaml"], guard.blocked["private/status.yaml"] = true, true
	read, err := reader.ReadComparisonBasis(context.Background(), lookup, basis)
	if err != nil || !read.Complete || len(read.Units) != 1 || read.Units[0].Address.MemberKey != "order_id" {
		t.Fatalf("root comparison escaped slice: %#v %v", read, err)
	}
	basis.Reference = "urn:kc:object:table/absent"
	empty, err := reader.ReadComparisonBasis(context.Background(), lookup, basis)
	if err != nil || !empty.Complete || len(empty.Units) != 0 || len(empty.Basis.Candidates) != 1 {
		t.Fatalf("proved absence lost candidate or became incomplete: %#v %v", empty, err)
	}
}
