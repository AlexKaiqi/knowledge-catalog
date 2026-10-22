package dataset_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func awaitPublicationAdmission(t *testing.T, admissions <-chan catalog.PublicationAdmission, done <-chan error) catalog.PublicationAdmission {
	t.Helper()
	select {
	case admission := <-admissions:
		return admission
	case err := <-done:
		t.Fatalf("publisher stopped before admission: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not reach admission")
	}
	return catalog.PublicationAdmission{}
}

func TestPublicationReceiptIdentifiesOneWinnerAmongEqualCandidates(t *testing.T) {
	f := newPublicationFixture(t)
	ctx := context.Background()
	lookup := catalog.PublicationLookup{SetID: f.candidate.SetID, Revision: f.candidate.Revision}
	var admissions [2]chan catalog.PublicationAdmission
	var release [2]chan struct{}
	var done [2]chan error
	for i, service := range []catalog.Service{f.service, f.peer} {
		admissions[i], release[i], done[i] = make(chan catalog.PublicationAdmission, 1), make(chan struct{}), make(chan error, 1)
		t.Cleanup(func() {
			select {
			case <-release[i]:
			default:
				close(release[i])
			}
		})
		go func() {
			_, err := publicationCoordinator(t, service).PublishDatasetGuarded(ctx, f.candidate, func(_ context.Context, admission catalog.PublicationAdmission) error {
				admissions[i] <- admission
				<-release[i]
				return nil
			})
			done[i] <- err
		}()
	}
	first := awaitPublicationAdmission(t, admissions[0], done[0])
	second := awaitPublicationAdmission(t, admissions[1], done[1])
	if first.ID == "" || first.ID == second.ID || first.PublicationAuthority != second.PublicationAuthority {
		t.Fatalf("equal candidates did not get distinct attempts at one authority version: %#v %#v", first, second)
	}
	if _, err := publicationCoordinator(t, f.peer).PublicationReceipt(ctx, lookup); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unaccepted candidate already has receipt: %v", err)
	}
	close(release[0])
	if err := <-done[0]; err != nil {
		t.Fatal(err)
	}
	close(release[1])
	if err := <-done[1]; kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("second equal attempt was accepted: %v", err)
	}
	receipt, err := publicationCoordinator(t, f.peer).PublicationReceipt(ctx, lookup)
	if err != nil || receipt.Admission != first.ID || receipt.Authority != first.PublicationAuthority || receipt.Definition != kernel.CanonicalDigest(f.candidate) {
		t.Fatalf("receipt did not identify the actual winner: %#v %v", receipt, err)
	}
	current, _ := f.peer.Set(f.candidate.SetID)
	if current.Revision != 1 {
		t.Fatal("receipt read silently refreshed service state")
	}
	checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(ctx, lookup, lookup)
	if err != nil || len(checkpoint.Receipts) != 1 || !reflect.DeepEqual(checkpoint.Receipts[0], receipt) {
		t.Fatalf("fixed checkpoint lost immutable winner: %#v %v", checkpoint, err)
	}
}

func TestPublicationReceiptAndDatasetSurviveLostAtomicCommitResponse(t *testing.T) {
	f := newPublicationFixture(t)
	lost := errors.New("actual atomic Dataset/receipt commit response lost")
	f.a.after = func(snapshot.TreeChangeSet) error { return lost }
	var admission catalog.PublicationAdmission
	_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(_ context.Context, attempt catalog.PublicationAdmission) error { admission = attempt; return nil })
	if !errors.Is(err, lost) {
		t.Fatalf("publication response was not lost: %v", err)
	}
	lookup := catalog.PublicationLookup{SetID: f.candidate.SetID, Revision: f.candidate.Revision}
	checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(context.Background(), lookup)
	if err != nil || len(checkpoint.Receipts) != 1 || checkpoint.Receipts[0].Admission != admission.ID {
		t.Fatalf("accepted receipt was lost after response failure: %#v %v", checkpoint, err)
	}
	accepted, err := f.peer.DatasetVersion(f.candidate.SetID, f.candidate.Revision)
	if err != nil || !reflect.DeepEqual(accepted, f.candidate) {
		t.Fatalf("receipt became visible without exact Dataset acceptance: %#v %v", accepted, err)
	}
}

type corruptPublicationReceipt struct{ *publicationAuthorityStore }

func (s corruptPublicationReceipt) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if strings.HasPrefix(path, "_publication/receipts/") {
		return []byte(`{"version":99,"admission":"corrupt"}`), nil
	}
	return s.publicationAuthorityStore.ReadFile(path, commit)
}

func TestCorruptPublicationReceiptCannotProduceRecoveryCheckpoint(t *testing.T) {
	f := newPublicationFixture(t)
	if _, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { return nil }); err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.OpenSnapshotRegistry(corruptPublicationReceipt{f.a}, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(f.sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	lookup := catalog.PublicationLookup{SetID: f.candidate.SetID, Revision: f.candidate.Revision}
	before, _ := f.a.Head(snapshot.DefaultRef)
	checkpoint, err := publicationCoordinator(t, service).FencePublications(context.Background(), lookup)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || checkpoint.Authority.Head != "" || len(checkpoint.Receipts) != 0 {
		t.Fatalf("bad receipt produced completion checkpoint: %#v %v", checkpoint, err)
	}
	if after, _ := f.a.Head(snapshot.DefaultRef); after != before {
		t.Fatal("bad receipt entered recovery mutation")
	}
	if _, err := publicationCoordinator(t, service.ReadView(nil)).PublicationReceipt(context.Background(), lookup); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("bad receipt was accepted by pure read: %v", err)
	}
}

type unconfirmedRegistryCommit struct {
	*publicationAuthorityStore
	unchanged bool
}

func (s unconfirmedRegistryCommit) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	if s.unchanged {
		return change.BaseCommit, nil
	}
	return "", nil
}

func TestPublicationReceiptRequiresConfirmedImmutableCommitVersion(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(fmt.Sprintf("unchanged=%t", unchanged), func(t *testing.T) {
			f := newPublicationFixture(t)
			registry, err := catalog.OpenSnapshotRegistry(unconfirmedRegistryCommit{publicationAuthorityStore: f.a, unchanged: unchanged}, snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			service, err := catalog.NewService(f.sources, registry)
			if err != nil {
				t.Fatal(err)
			}
			_, err = publicationCoordinator(t, service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { return nil })
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
				t.Fatalf("unconfirmed commit reported acceptance: %v", err)
			}
			assertUnpublishedCandidate(t, service)
			if _, err := publicationCoordinator(t, service).PublicationReceipt(context.Background(), catalog.PublicationLookup{SetID: f.candidate.SetID, Revision: f.candidate.Revision}); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
				t.Fatalf("unconfirmed commit exposed accepted receipt: %v", err)
			}
		})
	}
}

type pausedPublicationReceiptRead struct {
	*publicationAuthorityStore
	paused  atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (s *pausedPublicationReceiptRead) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if strings.HasPrefix(path, "_publication/receipts/") && s.paused.CompareAndSwap(false, true) {
		close(s.entered)
		<-s.release
	}
	return s.publicationAuthorityStore.ReadFile(path, commit)
}

func TestPublicationReceiptPreflightDoesNotHoldCommitLocks(t *testing.T) {
	f := newPublicationFixture(t)
	store := &pausedPublicationReceiptRead{publicationAuthorityStore: f.a, entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-store.release:
		default:
			close(store.release)
		}
	})
	registry, err := catalog.OpenSnapshotRegistry(store, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(f.sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	published := make(chan error, 1)
	go func() {
		_, err := publicationCoordinator(t, service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { return nil })
		published <- err
	}()
	select {
	case <-store.entered:
	case err := <-published:
		t.Fatalf("receipt preflight was not reached: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("receipt preflight did not start")
	}
	registered := make(chan error, 1)
	go func() { registered <- service.RegisterRepository("kr://publication/other") }()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("receipt IO held a commit lock across remote preflight")
	}
	close(store.release)
	if err := <-published; kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("preflight rebased after concurrent authority change: %v", err)
	}
	assertUnpublishedCandidate(t, service)
}
