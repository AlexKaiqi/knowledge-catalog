package reader_test

import (
	"context"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// A → B → A restores the previous local value but is not the same revision.
// The promise has three parts and they must hold together: the basis taken at
// the first A stays readable at its own commit, an independent request binds the
// third commit rather than reusing the old coordinates, and the history keeps
// both real changes instead of collapsing them into one.
func TestReturnToEarlierValueKeepsFirstBasisNewRequestAndBothRevisions(t *testing.T) {
	_, guard, w, first := newReferenceReadFixture(t)
	ref := objectmodel.Reference{Object: "table/orders", Locator: "/columns/order_id/constraints/nullable"}
	nullable := func(value bool) map[string]any {
		return map[string]any{"constraints": map[string]any{"nullable": value}}
	}
	put := func(command string, base kernel.CommitID, value bool) kernel.CommitID {
		t.Helper()
		receipt, err := w.Commit(command, objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut,
				Address:  objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: ref.Object, AspectName: "columns", MemberKey: "order_id"},
				PathHint: "public/id.yaml",
				Value:    nullable(value)}}})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.Result.NewCommit
	}
	bound := &basisSnapshot{referenceReadGuard: guard, binding: "authority-a"}
	repo, err := reader.NewReader(nil).Wrap(bound, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != bound.ID() {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "unknown repository")
		}
		return repo, nil
	}
	pinned := func(commit kernel.CommitID) reader.DatasetPin {
		return reader.DatasetPin{SetID: "orders", Revision: 3,
			Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
			Items:        []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "file", File: "public/id.yaml"}}}
	}

	// The first A: the value the fixture seeded, read through a frozen selection.
	basis, err := reader.Open(lookup, pinned(first)).ReadBasis(ref, "")
	if err != nil {
		t.Fatal(err)
	}

	// B changes the value, then A restores it. The third commit is not the first.
	second := put("roundtrip-to-b", first, true)
	third := put("roundtrip-back-to-a", second, false)
	if third == first || third == second {
		t.Fatalf("returning to the earlier value reused a commit: first=%s second=%s third=%s", first, second, third)
	}

	// The first A's basis still reads its own coordinates, not the restored head.
	replayed, err := reader.OpenReadBasis(lookup, basis)
	if err != nil {
		t.Fatal(err)
	}
	readRef := ref
	readRef.Repository = repo.ID()
	values, err := replayed.ReadReference(readRef)
	if err != nil || len(values) != 1 || values[0].Value != false || values[0].Commit != first {
		t.Fatalf("first basis did not survive the round trip: %#v %v", values, err)
	}
	if got := replayed.Pin().Repositories[repo.ID()]; got != first {
		t.Fatalf("fixed replay moved to the restored commit: %s", got)
	}

	// An independent request resolves the same value at the third commit, so
	// reuse cannot be claimed by keeping the old coordinates.
	fresh, err := reader.Open(lookup, pinned(third)).ReadReference(readRef)
	if err != nil || len(fresh) != 1 || fresh[0].Value != false || fresh[0].Commit != third {
		t.Fatalf("independent request did not bind the third commit: %#v %v", fresh, err)
	}

	// Both changes stay in history: the restored value does not erase the detour.
	history, ok := snapshot.HistoryStoreOf(guard.Store)
	if !ok {
		t.Fatal("fixture has no actual commit history")
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
		t.Fatal(err)
	}
	scanner, err := reader.NewReader(registry).Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.(objectmodel.HistoryScanner).ScanHistory(context.Background(), objectmodel.ObjectID(ref.Object), third,
		objectmodel.HistoryScanQuery{Limit: 10, Budget: 32})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Complete {
		t.Fatalf("history page did not complete: %#v", page)
	}
	var seen []kernel.CommitID
	for _, revision := range page.Revisions {
		seen = append(seen, revision.Commit)
	}
	if len(seen) != 3 || seen[0] != third || seen[1] != second || seen[2] != first {
		t.Fatalf("history collapsed the round trip: %v", seen)
	}
}
