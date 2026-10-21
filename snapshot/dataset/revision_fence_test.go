package dataset_test

import (
	"errors"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

// Work bound to the current revision and a publication of the same Dataset
// are ordered at the commit: the publication waits for the work, while reads
// and other Datasets do not.
func TestWithCurrentRevisionOrdersPublicationOfTheSameDataset(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://fence/catalog")
	cat := catalogFromRegistry(t, registry, "kr://fence/source")
	if err := cat.RegisterRepository("kr://fence/source"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "kr://fence/source", Selector: snapshot.DefaultRef}}
	for _, id := range []string{"task", "other"} {
		if _, err := cat.DefineDataset(id, 1, sources); err != nil {
			t.Fatal(err)
		}
	}
	entered, release, held := holdRevision(t, cat, "task", 1)
	<-entered
	published := make(chan error, 1)
	go func() {
		_, err := cat.RequestView(nil, "alice", "req-publish", "").DefineDataset("task", 2, sources)
		published <- err
	}()
	within(t, "reads and other Datasets during held revision work", func() {
		if got, err := cat.Set("task"); err != nil || got.Revision != 1 {
			t.Errorf("read during held work served %d %v", got.Revision, err)
		}
		if _, err := cat.DefineDataset("other", 2, sources); err != nil {
			t.Errorf("another Dataset waited or failed: %v", err)
		}
	})
	expectWaiting(t, published, "publication")
	close(release)
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	entered, release, held = holdRevision(t, cat, "task", 2)
	<-entered
	retired := make(chan error, 1)
	go func() { retired <- cat.RetireDataset("task") }()
	expectWaiting(t, retired, "retirement")
	close(release)
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	if err := <-retired; err != nil {
		t.Fatal(err)
	}
	if got, err := cat.Set("task"); err != nil || got.Revision != 2 || !got.Retired {
		t.Fatalf("waiting writes were not applied after the work: %#v %v", got, err)
	}
}

func holdRevision(t *testing.T, cat catalog.Service, setID string, revision int) (chan struct{}, chan struct{}, chan error) {
	t.Helper()
	entered, release, held := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		held <- cat.WithCurrentRevision(setID, revision, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	return entered, release, held
}

func expectWaiting(t *testing.T, done <-chan error, what string) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s of the same Dataset landed inside held revision work: %v", what, err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWithCurrentRevisionRejectsWithoutRunning(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://fence-reject/catalog")
	cat := catalogFromRegistry(t, registry, "kr://fence-reject/source")
	if err := cat.RegisterRepository("kr://fence-reject/source"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "kr://fence-reject/source", Selector: snapshot.DefaultRef}}
	for revision := 1; revision <= 2; revision++ {
		if _, err := cat.DefineDataset("task", revision, sources); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cat.DefineDataset("gone", 1, sources); err != nil {
		t.Fatal(err)
	}
	if err := cat.RetireDataset("gone"); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]struct {
		setID    string
		revision int
	}{"superseded": {"task", 1}, "future": {"task", 3}, "retired": {"gone", 1}, "absent": {"missing", 1}} {
		ran := false
		err := cat.WithCurrentRevision(call.setID, call.revision, func() error { ran = true; return nil })
		if ran || kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("%s: ran=%v err=%v", name, ran, err)
		}
	}
	problem := errors.New("accept failed")
	if err := cat.WithCurrentRevision("task", 2, func() error { return problem }); !errors.Is(err, problem) {
		t.Fatalf("work error was not returned: %v", err)
	}
	if err := cat.WithCurrentRevision("task", 2, nil); err != nil {
		t.Fatalf("plain current check: %v", err)
	}
}
