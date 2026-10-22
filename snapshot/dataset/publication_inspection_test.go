package dataset_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestPublicationInspectionReadsActualHeadWithoutFencingPausedPublisher(t *testing.T) {
	f := newPublicationFixture(t)
	inspector := f.peer.(dataset.PublicationInspector)
	admitted := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 1)
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
	})
	go func() {
		_, err := f.service.(dataset.PublicationCoordinator).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, dataset.PublicationAdmission) error { close(admitted); <-resume; return nil })
		done <- err
	}()
	select {
	case <-admitted:
	case err := <-done:
		t.Fatalf("publisher did not reach admission: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("admission timeout")
	}
	before, _ := f.authority.Head(snapshot.DefaultRef)
	checkpoint, err := inspector.InspectPublications(context.Background(), dataset.PublicationLookup{SetID: "notes", Revision: 2})
	after, _ := f.authority.Head(snapshot.DefaultRef)
	if err != nil || checkpoint.Authority.Head != before || after != before || len(checkpoint.Receipts) != 0 {
		t.Fatalf("inspection fenced/accepted a pending writer: %#v %v", checkpoint, err)
	}
	close(resume)
	if err := <-done; err != nil {
		t.Fatalf("read-only inspection invalidated original publication CAS: %v", err)
	}
	checkpoint, err = inspector.InspectPublications(context.Background(), dataset.PublicationLookup{SetID: "notes", Revision: 2})
	if err != nil || checkpoint.Authority.Head == before || len(checkpoint.Receipts) != 1 || checkpoint.Receipts[0].Revision != 2 {
		t.Fatalf("inspection read cached authority: %#v %v", checkpoint, err)
	}
	if stale, err := f.peer.Set("notes"); err != nil || stale.Revision != 1 {
		t.Fatalf("inspection changed peer's frozen accepted view: %#v %v", stale, err)
	}
	checkpoint.State.Datasets[0].SetID = "caller-mutation"
	again, err := inspector.InspectPublications(context.Background())
	if err != nil || len(again.State.Datasets) != 1 || again.State.Datasets[0].SetID != "notes" {
		t.Fatalf("inspection values alias caller memory: %#v %v", again, err)
	}
}

func TestPublicationInspectionHonorsOriginalReadContextAndNeverInitializes(t *testing.T) {
	f := newPublicationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	view, err := f.service.(dataset.ReadContextBound).ReadContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.authority.Head(snapshot.DefaultRef)
	cancel()
	checkpoint, err := view.(dataset.PublicationInspector).InspectPublications(context.Background())
	after, _ := f.authority.Head(snapshot.DefaultRef)
	if !errors.Is(err, context.Canceled) || checkpoint.Authority.Head != "" || checkpoint.State.AuthorityID != "" || after != before {
		t.Fatalf("inspection escaped cancellation or mutated authority: %#v %v", checkpoint, err)
	}
	checkpoint, err = f.service.(dataset.PublicationInspector).InspectPublications(context.Background(), dataset.PublicationLookup{SetID: "notes", Revision: 1})
	if err != nil || checkpoint.Authority.Head != before || checkpoint.State.AuthorityID != string(checkpoint.Authority.AuthorityID) {
		t.Fatalf("canceled view poisoned owner: %#v %v", checkpoint, err)
	}
	if _, err := f.service.(dataset.PublicationInspector).InspectPublications(context.Background(), dataset.PublicationLookup{SetID: "notes", Revision: -1}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid inspection lookup initialized a record: %v", err)
	}
	after, _ = f.authority.Head(snapshot.DefaultRef)
	if after != before {
		t.Fatal("failed inspection wrote authority")
	}
}
