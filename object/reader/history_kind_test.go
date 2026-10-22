package reader_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestObjectHistoryRetainsKindOnlyRevisionAtSameMaintenanceCoordinates(t *testing.T) {
	_, guard, w, base := newReferenceReadFixture(t)
	put := func(command string, at kernel.CommitID, kind objectmodel.AddressKind) kernel.CommitID {
		t.Helper()
		receipt, err := w.Commit(command, objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: kind, ObjectID: "table/orders", AspectName: "description"}, Value: map[string]any{"summary": "orders"}}}})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.Result.NewCommit
	}
	aspect := put("history-aspect", base, objectmodel.KindAspect)
	record := put("history-record", aspect, objectmodel.KindRecord)
	history, ok := snapshot.HistoryStoreOf(guard.Store)
	if !ok {
		t.Fatal("kind history fixture must expose real commit history")
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(&comparisonHistoryStore{referenceReadGuard: guard, HistoryStore: history}); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(stores)
	revisions, err := rd.Log(guard.ID(), "table/orders", record, objectmodel.ObjectLogQuery{Limit: 2})
	if err != nil || len(revisions) != 2 || revisions[0].Commit != record || revisions[1].Commit != aspect {
		t.Fatalf("kind-only revision was folded away: %#v %v", revisions, err)
	}
	if revisions[0].Digest != revisions[1].Digest || revisions[0].DeclarationDigest != revisions[1].DeclarationDigest {
		t.Fatal("fixture must change only complete unit kind, not value or declaration")
	}
}

type comparisonHistoryStore struct {
	*referenceReadGuard
	snapshot.HistoryStore
}
