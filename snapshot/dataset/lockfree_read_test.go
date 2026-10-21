package dataset_test

import (
	"sync"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

// A registry write held at the authority commit must not delay any read:
// readers serve the accepted version they load and see the new one only after
// the commit is accepted. Preparing another publication reads immutable
// commits and does not wait either.
func TestCatalogReadsDoNotWaitForAHeldPublication(t *testing.T) {
	authority, registry := createCatalogRegistry(t, "kr://lockfree/catalog")
	cat := catalogFromRegistry(t, registry, "kr://lockfree/source")
	if err := cat.RegisterRepository("kr://lockfree/source"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "kr://lockfree/source", Selector: snapshot.DefaultRef}}
	if _, err := cat.DefineDataset("task", 1, sources); err != nil {
		t.Fatal(err)
	}
	before := cat.ReadView(nil)
	entered, release := holdNextCommit(authority)
	published := make(chan error, 1)
	go func() {
		_, err := cat.RequestView(nil, "alice", "req-publish", "").DefineDataset("task", 2, sources)
		published <- err
	}()
	<-entered

	within(t, "reads during a held publication", func() {
		for name, view := range map[string]catalog.Service{"shared": cat, "read view": cat.ReadView(nil), "request view": cat.RequestView(nil, "bob", "req-read", "")} {
			if got, err := view.Set("task"); err != nil || got.Revision != 1 {
				t.Errorf("%s served %d %v before the commit was accepted", name, got.Revision, err)
			}
			if _, err := view.DatasetVersion("task", 1); err != nil {
				t.Errorf("%s version read: %v", name, err)
			}
			if !view.HasRepository("kr://lockfree/source") || len(view.DumpState().Datasets) != 1 {
				t.Errorf("%s lost the accepted registry state", name)
			}
			if _, err := view.Log(catalog.LogQuery{Limit: 5}); err != nil {
				t.Errorf("%s log: %v", name, err)
			}
		}
		if _, err := registry.Head(); err != nil {
			t.Error(err)
		}
		if err := registry.CheckAuthority(); err != nil {
			t.Errorf("authority check during a held commit: %v", err)
		}
		if _, err := cat.PrepareDataset("other", 1, sources); err != nil {
			t.Errorf("prepare waited or failed: %v", err)
		}
	})

	close(release)
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	if got, err := cat.Set("task"); err != nil || got.Revision != 2 {
		t.Fatalf("accepted publication is not visible: %d %v", got.Revision, err)
	}
	if got, err := before.Set("task"); err != nil || got.Revision != 1 {
		t.Fatalf("read view moved with a later publication: %d %v", got.Revision, err)
	}
}

// Validation runs before the commit lock, so a publication validated against
// an older version is rechecked at commit: two publications of one revision
// leave exactly one accepted release.
func TestCatalogConcurrentPublicationsOfOneRevisionAcceptOne(t *testing.T) {
	authority, registry := createCatalogRegistry(t, "kr://lockfree-race/catalog")
	cat := catalogFromRegistry(t, registry, "kr://lockfree-race/source")
	if err := cat.RegisterRepository("kr://lockfree-race/source"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "kr://lockfree-race/source", Selector: snapshot.DefaultRef}}
	if _, err := cat.DefineDataset("task", 1, sources); err != nil {
		t.Fatal(err)
	}
	first, err := cat.PrepareDataset("task", 2, sources)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cat.PrepareDataset("task", 2, sources)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := holdNextCommit(authority)
	results := make(chan error, 2)
	go func() { _, err := cat.PublishDataset(first); results <- err }()
	<-entered
	go func() { _, err := cat.PublishDataset(second); results <- err }()
	close(release)
	accepted, rejected := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			accepted++
		case kernel.CodeOf(err) == kernel.ErrNonFastForward:
			rejected++
		default:
			t.Fatalf("unexpected publication error: %v", err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d, want exactly one release", accepted, rejected)
	}
	if got, err := cat.Set("task"); err != nil || got.Revision != 2 {
		t.Fatalf("accepted release = %d %v", got.Revision, err)
	}
}

// holdNextCommit parks the next authority commit until release is closed;
// later commits pass straight through.
func holdNextCommit(authority *catalogMem) (<-chan struct{}, chan struct{}) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	authority.beforeApply = func() {
		held := false
		once.Do(func() { held = true; close(entered) })
		if held {
			<-release
		}
	}
	return entered, release
}

func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waited for a held registry write", what)
	}
}
