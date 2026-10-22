package dataset_test

import (
	"context"
	"errors"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func publicationAdopter(t *testing.T, service catalog.Service) catalog.PublicationAdopter {
	t.Helper()
	adopter, ok := service.(catalog.PublicationAdopter)
	if !ok {
		t.Fatal("file authority does not support preparation adoption")
	}
	return adopter
}

func TestPublicationAdoptionPreservesImmutableReleaseAndIsIdempotent(t *testing.T) {
	f := newPublicationFixture(t)
	original, err := f.service.PublishDataset(f.candidate)
	if err != nil {
		t.Fatal(err)
	}
	before := kernel.CanonicalDigest(catalog.NormalizeState(f.service.DumpState()))
	key := catalog.PublicationLookup{SetID: original.SetID, Revision: original.Revision}
	calls := 0
	var admitted catalog.PublicationAdmission
	accepted, err := publicationAdopter(t, f.service).AdoptPublication(context.Background(), key, func(_ context.Context, def catalog.Dataset, admission catalog.PublicationAdmission) error {
		calls++
		admitted = admission
		if kernel.CanonicalDigest(def) != kernel.CanonicalDigest(original) {
			t.Fatal("adoption changed accepted candidate")
		}
		def.Metadata = map[string]string{"attempt": "must not alter release"}
		return nil
	})
	if err != nil || kernel.CanonicalDigest(accepted) != kernel.CanonicalDigest(original) {
		t.Fatalf("adoption result changed release: %#v %v", accepted, err)
	}
	if before != kernel.CanonicalDigest(catalog.NormalizeState(f.service.DumpState())) {
		t.Fatal("preparation adoption changed registry definitions or revisions")
	}
	receipt, err := publicationCoordinator(t, f.peer).PublicationReceipt(context.Background(), key)
	if err != nil || receipt.Admission != admitted.ID || receipt.Authority != admitted.PublicationAuthority || receipt.Definition != kernel.CanonicalDigest(original) {
		t.Fatalf("adoption did not persist exact preparation identity: %#v %v", receipt, err)
	}
	_, err = publicationAdopter(t, f.peer).AdoptPublication(context.Background(), key, func(context.Context, catalog.Dataset, catalog.PublicationAdmission) error { calls++; return nil })
	if err != nil || calls != 1 {
		t.Fatalf("existing preparation was replaced: calls=%d err=%v", calls, err)
	}
	again, err := publicationCoordinator(t, f.peer).PublicationReceipt(context.Background(), key)
	if err != nil || again != receipt {
		t.Fatalf("idempotent adoption changed immutable receipt: %#v %v", again, err)
	}
}

func TestPublicationAdoptionFencesEarlierConcurrentPreparation(t *testing.T) {
	f := newPublicationFixture(t)
	def, err := f.service.PublishDataset(f.candidate)
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	admitted := make(chan catalog.PublicationAdmission, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	go func() {
		_, err := publicationAdopter(t, f.service).AdoptPublication(context.Background(), key, func(_ context.Context, _ catalog.Dataset, admission catalog.PublicationAdmission) error {
			admitted <- admission
			<-release
			return nil
		})
		done <- err
	}()
	first := awaitPublicationAdmission(t, admitted, done)
	var winner catalog.PublicationAdmission
	_, err = publicationAdopter(t, f.peer).AdoptPublication(context.Background(), key, func(_ context.Context, _ catalog.Dataset, admission catalog.PublicationAdmission) error {
		winner = admission
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("late old migration was accepted: %v", err)
	}
	receipt, err := publicationCoordinator(t, f.peer).PublicationReceipt(context.Background(), key)
	if err != nil || receipt.Admission != winner.ID || winner.ID == first.ID {
		t.Fatalf("migration winner was ambiguous: %#v %v", receipt, err)
	}
	actual, err := f.peer.DatasetVersion(key.SetID, key.Revision)
	if err != nil || kernel.CanonicalDigest(actual) != kernel.CanonicalDigest(def) {
		t.Fatalf("migration changed accepted content: %#v %v", actual, err)
	}
}

func TestPublicationAdoptionLostCommitResponseRetainsActualReceipt(t *testing.T) {
	f := newPublicationFixture(t)
	def, err := f.service.PublishDataset(f.candidate)
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	lost := errors.New("adoption receipt commit accepted but response lost")
	var admitted catalog.PublicationAdmission
	_, err = publicationAdopter(t, f.service).AdoptPublication(context.Background(), key, func(_ context.Context, _ catalog.Dataset, admission catalog.PublicationAdmission) error {
		admitted = admission
		f.a.after = func(snapshot.TreeChangeSet) error { return lost }
		return nil
	})
	if !errors.Is(err, lost) {
		t.Fatalf("adoption commit was not uncertain: %v", err)
	}
	receipt, err := publicationCoordinator(t, f.peer).PublicationReceipt(context.Background(), key)
	if err != nil || receipt.Admission != admitted.ID {
		t.Fatalf("actual adoption was lost: %#v %v", receipt, err)
	}
}
