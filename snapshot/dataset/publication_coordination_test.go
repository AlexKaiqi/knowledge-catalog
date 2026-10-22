package dataset_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

type publicationAuthorityStore struct {
	*catalogMem
	origin string
	before func(snapshot.TreeChangeSet)
	after  func(snapshot.TreeChangeSet) error
}

func (s *publicationAuthorityStore) Origin() string { return s.origin }

type reboundPublicationStore struct {
	*publicationAuthorityStore
	replacement snapshot.Store
}

func (s *reboundPublicationStore) WithContext(context.Context) snapshot.Store {
	return s.replacement
}

func (s *publicationAuthorityStore) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	if s.before != nil {
		s.before(change)
	}
	commit, err := s.catalogMem.ApplyTreeCommit(change)
	if err == nil && s.after != nil {
		err = s.after(change)
	}
	return commit, err
}

type publicationFixture struct {
	authority     *catalogMem
	a, b          *publicationAuthorityStore
	service, peer catalog.Service
	sources       snapshot.Registry
	candidate     catalog.Dataset
}

func newPublicationFixture(t *testing.T) publicationFixture {
	t.Helper()
	authority := newCatalogMem("kr://publication/authority")
	a := &publicationAuthorityStore{catalogMem: authority, origin: "https://authority.example"}
	b := &publicationAuthorityStore{catalogMem: authority, origin: a.origin}
	registry, err := catalog.CreateSnapshotRegistry(a, "")
	if err != nil {
		t.Fatal(err)
	}
	sources := snapshot.NewRegistry()
	for _, id := range []string{"kr://publication/source", "kr://publication/other"} {
		if err := sources.Add(testkit.MakeRepository(t, id)); err != nil {
			t.Fatal(err)
		}
	}
	service, err := catalog.NewService(sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterRepository("kr://publication/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DefineDataset("notes", 1, []catalog.DatasetSource{{Repository: "kr://publication/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	candidate, err := service.PrepareDataset("notes", 2, []catalog.DatasetSource{{Repository: "kr://publication/source", Selector: snapshot.DefaultRef}})
	if err != nil {
		t.Fatal(err)
	}
	peerRegistry, err := catalog.OpenSnapshotRegistry(b, "")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := catalog.NewService(sources, peerRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return publicationFixture{authority: authority, a: a, b: b, service: service, peer: peer, sources: sources, candidate: candidate}
}

func publicationCoordinator(t *testing.T, service catalog.Service) catalog.PublicationCoordinator {
	t.Helper()
	coordinator, ok := service.(catalog.PublicationCoordinator)
	if !ok {
		t.Fatal("file authority has no publication coordinator")
	}
	return coordinator
}

func assertUnpublishedCandidate(t *testing.T, service catalog.Service) {
	t.Helper()
	current, err := service.Set("notes")
	if err != nil || current.Revision != 1 {
		t.Fatalf("old publication moved: %#v %v", current, err)
	}
	if _, err := service.DatasetVersion("notes", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unaccepted candidate gained a release: %v", err)
	}
}

func TestPublicationAdmissionRunsOutsideCommitLockAndCannotRebase(t *testing.T) {
	f := newPublicationFixture(t)
	admitted, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error {
			close(admitted)
			<-release
			return nil
		})
		done <- err
	}()
	select {
	case <-admitted:
	case <-time.After(5 * time.Second):
		t.Fatal("admission did not run")
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	registered := make(chan error, 1)
	go func() { registered <- f.service.RegisterRepository("kr://publication/other") }()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external admission blocked an unrelated registry commit")
	}
	assertUnpublishedCandidate(t, f.service)
	close(release)
	if err := <-done; kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("publication silently rebased admission: %v", err)
	}
	assertUnpublishedCandidate(t, f.service)
}

func TestPublicationFenceRejectsResumedAndAlreadyDispatchedOldPublishers(t *testing.T) {
	for _, position := range []string{"admission", "authority-CAS"} {
		t.Run(position, func(t *testing.T) {
			f := newPublicationFixture(t)
			paused, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			if position == "authority-CAS" {
				f.a.before = func(change snapshot.TreeChangeSet) {
					if strings.HasPrefix(change.Message, "dataset-define ") {
						close(paused)
						<-release
					}
				}
			}
			go func() {
				_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error {
					if position == "admission" {
						close(paused)
						<-release
					}
					return nil
				})
				done <- err
			}()
			select {
			case <-paused:
			case <-time.After(5 * time.Second):
				t.Fatal("publisher did not pause at the intended boundary")
			}
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			before := f.peer.DumpState()
			oldHead, _ := f.authority.Head(snapshot.DefaultRef)
			checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(context.Background())
			if err != nil || checkpoint.Authority.Head == oldHead || checkpoint.Authority.Binding == "" || !reflect.DeepEqual(checkpoint.State, before) {
				t.Fatalf("fence did not preserve accepted content with a new authority coordinate: %#v %v", checkpoint, err)
			}
			close(release)
			if err := <-done; kernel.CodeOf(err) != kernel.ErrNonFastForward {
				t.Fatalf("old publisher crossed the accepted fence: %v", err)
			}
			if head, _ := f.authority.Head(snapshot.DefaultRef); head != checkpoint.Authority.Head {
				t.Fatal("late publisher changed the fenced authority")
			}
			assertUnpublishedCandidate(t, f.peer)
		})
	}
}

func TestPublicationFenceRecoversActuallyAcceptedLostPublicationResponse(t *testing.T) {
	f := newPublicationFixture(t)
	f.a.after = func(change snapshot.TreeChangeSet) error {
		if strings.HasPrefix(change.Message, "dataset-define ") {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "lost accepted publication response")
		}
		return nil
	}
	_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { return nil })
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("accepted response loss was not exposed: %v", err)
	}
	assertUnpublishedCandidate(t, f.peer) // The peer is a stale, independent handle.
	checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, version := range checkpoint.State.DatasetVersions {
		if version.SetID == "notes" && version.Revision == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("cached absence was mistaken for authoritative non-adoption: %#v", checkpoint)
	}
	if current, err := f.peer.Set("notes"); err != nil || current.Revision != 2 {
		t.Fatalf("fence did not recover accepted state: %#v %v", current, err)
	}
}

func TestPublicationFenceLostResponseReturnsNoCheckpointAndCanRetry(t *testing.T) {
	f := newPublicationFixture(t)
	var lost atomic.Bool
	f.b.after = func(change snapshot.TreeChangeSet) error {
		if change.Message == "dataset-publication-fence" && lost.CompareAndSwap(false, true) {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "lost accepted fence response")
		}
		return nil
	}
	checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(context.Background())
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || checkpoint.Authority.Head != "" || !lost.Load() {
		t.Fatalf("unknown fence produced completion proof: %#v %v", checkpoint, err)
	}
	actual, _ := f.authority.Head(snapshot.DefaultRef)
	recovered, err := publicationCoordinator(t, f.peer).FencePublications(context.Background())
	if err != nil || recovered.Authority.Head == actual || !reflect.DeepEqual(recovered.State, f.service.DumpState()) {
		t.Fatalf("fence could not recover from actual authority: %#v %v", recovered, err)
	}
}

func TestPublicationCoordinationRechecksReadOnlyAndCancellation(t *testing.T) {
	f := newPublicationFixture(t)
	readOnly := publicationCoordinator(t, f.service.ReadView(nil))
	called := false
	if _, err := readOnly.PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { called = true; return nil }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || called {
		t.Fatalf("read view admitted a publication: %v", err)
	}
	if _, err := readOnly.FencePublications(context.Background()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("read view fenced authority: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(ctx, f.candidate, func(context.Context, catalog.PublicationAdmission) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission reached authority: %v", err)
	}
	assertUnpublishedCandidate(t, f.service)
}

func TestPublicationCoordinationRequiresInstanceBinding(t *testing.T) {
	f := newPublicationFixture(t)
	registry, err := catalog.OpenSnapshotRegistry(f.authority, "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(f.sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := publicationCoordinator(t, service)
	before, _ := f.authority.Head(snapshot.DefaultRef)
	called := false
	if _, err := coordinator.PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error { called = true; return nil }); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || called {
		t.Fatalf("unbound authority admitted a protected publication: %v", err)
	}
	if _, err := coordinator.FencePublications(context.Background()); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unbound authority produced a fence: %v", err)
	}
	if after, _ := f.authority.Head(snapshot.DefaultRef); after != before {
		t.Fatal("binding failure moved authority")
	}
}

func TestPublicationCoordinationRejectsContextAuthorityRedirection(t *testing.T) {
	f := newPublicationFixture(t)
	other := newPublicationFixture(t)
	other.a.origin = "https://other-authority.example"
	store := &reboundPublicationStore{publicationAuthorityStore: f.a, replacement: other.a}
	registry, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(f.sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.authority.Head(snapshot.DefaultRef)
	otherBefore, _ := other.authority.Head(snapshot.DefaultRef)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coordinator := publicationCoordinator(t, service)
	if _, err := coordinator.FencePublications(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("redirected authority produced checkpoint: %v", err)
	}
	if _, err := coordinator.PublishDatasetGuarded(ctx, f.candidate, func(context.Context, catalog.PublicationAdmission) error { return nil }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("redirected authority accepted publication: %v", err)
	}
	if after, _ := f.authority.Head(snapshot.DefaultRef); after != before {
		t.Fatal("rejected redirection moved original authority")
	}
	if after, _ := other.authority.Head(snapshot.DefaultRef); after != otherBefore {
		t.Fatal("rejected redirection moved foreign authority")
	}
	assertUnpublishedCandidate(t, service)
}

func TestPublicationCoordinationRechecksBindingAfterAdmission(t *testing.T) {
	f := newPublicationFixture(t)
	before, _ := f.authority.Head(snapshot.DefaultRef)
	_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(context.Context, catalog.PublicationAdmission) error {
		f.a.origin = "https://replacement-authority.example"
		return nil
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("changed authority binding accepted publication: %v", err)
	}
	if after, _ := f.authority.Head(snapshot.DefaultRef); after != before {
		t.Fatal("binding changed after admission moved authority")
	}
	assertUnpublishedCandidate(t, f.service)
}

func TestPublicationFenceObserverFailureDoesNotExposeCheckpoint(t *testing.T) {
	f := newPublicationFixture(t)
	failed := errors.New("fence evidence persistence failed")
	f.peer.SetObserver(func(string, map[string]any, error) error { return failed })
	before, _ := f.authority.Head(snapshot.DefaultRef)
	checkpoint, err := publicationCoordinator(t, f.peer).FencePublications(context.Background())
	if !errors.Is(err, failed) || checkpoint.Authority.Head != "" {
		t.Fatalf("failed fence evidence exposed completion proof: %#v %v", checkpoint, err)
	}
	if actual, _ := f.authority.Head(snapshot.DefaultRef); actual == before {
		t.Fatal("test did not cross actual fence acceptance")
	}
	f.peer.SetObserver(nil)
	if _, err := publicationCoordinator(t, f.peer).FencePublications(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationFenceIdentityAndCASStayOnTheConfiguredRef(t *testing.T) {
	f := newPublicationFixture(t)
	before, _ := f.authority.Head(snapshot.DefaultRef)
	const otherRef = "refs/heads/other"
	if err := f.authority.CreateRef(otherRef, before); err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.OpenSnapshotRegistry(f.b, otherRef)
	if err != nil {
		t.Fatal(err)
	}
	other, err := catalog.NewService(f.sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	var admitted catalog.PublicationAdmission
	result, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(context.Background(), f.candidate, func(_ context.Context, authority catalog.PublicationAdmission) error {
		admitted = authority
		checkpoint, err := publicationCoordinator(t, other).FencePublications(context.Background())
		if err != nil {
			return err
		}
		if checkpoint.Authority.Ref != otherRef || checkpoint.Authority.Binding == authority.Binding {
			t.Fatal("different authority refs shared an admission identity")
		}
		if head, _ := f.authority.Head(snapshot.DefaultRef); head != before {
			t.Fatal("fencing another ref moved this publication's CAS target")
		}
		return nil
	})
	if err != nil || result.Revision != 2 || admitted.Ref != snapshot.DefaultRef {
		t.Fatalf("unrelated ref invalidated publication: %#v %v", result, err)
	}
	assertUnpublishedCandidate(t, other)
}

func TestPublicationCancellationDoesNotWaitForAnotherCommit(t *testing.T) {
	f := newPublicationFixture(t)
	paused, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	f.a.before = func(change snapshot.TreeChangeSet) {
		if strings.HasPrefix(change.Message, "dataset-define ") {
			close(paused)
			<-release
		}
	}
	go func() { _, err := f.service.PublishDataset(f.candidate); done <- err }()
	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("first commit did not pause")
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	next := f.candidate
	next.Revision = 3
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	admitted := make(chan struct{})
	go func() {
		_, err := publicationCoordinator(t, f.service).PublishDatasetGuarded(ctx, next, func(context.Context, catalog.PublicationAdmission) error { close(admitted); return nil })
		result <- err
	}()
	select {
	case <-admitted:
	case <-time.After(5 * time.Second):
		t.Fatal("next admission waited on another commit")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled publication lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled publication stayed behind another registry commit")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
