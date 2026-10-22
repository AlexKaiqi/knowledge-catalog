package home

import (
	"context"
	"path/filepath"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestBorrowedReadViewKeepsDurableBasisRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-bases.db")
	store, err := reader.NewReadBasisStore(path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := reader.NewHistoryCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ws := &Home{ReadBases: store, HistoryCheckpoints: history}
	view := ws.ReadView(nil)
	if view.ReadBases == nil || view.ReadBases != ws.ReadBases {
		t.Fatal("request lost the root's durable basis authority")
	}
	if view.HistoryCheckpoints == nil || view.HistoryCheckpoints != ws.HistoryCheckpoints {
		t.Fatal("request lost durable history progress")
	}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders",
		Candidates: []objectmodel.ReadSource{{Repository: "kr://trade", Commit: "fixed", Binding: "authority"}},
		Paths:      []objectmodel.ReadPath{{Repository: "kr://trade", Kind: "prefix"}}}
	reference, err := view.ReadBases.Retain(context.Background(), basis)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := objectmodel.HistoryCheckpoint{Version: 1, Basis: basis, Open: map[kernel.RepositoryID]*objectmodel.HistoryScanProgress{"kr://trade": nil}}
	continuation, err := view.HistoryCheckpoints.Retain(context.Background(), checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ws.ReadBases.Resolve(context.Background(), reference)
	if err != nil || got.Candidates[0].Commit != "fixed" {
		t.Fatalf("closing a request lost fixed coordinates: %#v %v", got, err)
	}
	if _, err := ws.HistoryCheckpoints.Resolve(context.Background(), continuation); err != nil {
		t.Fatalf("closing request lost history progress: %v", err)
	}
}
