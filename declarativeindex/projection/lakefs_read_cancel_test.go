package projection

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot"
)

// Goal: closing maintenance interrupts the one lakeFS read already in flight.
// The path under test is the real lakeFS adapter behind the real Reader, driven
// by Index.Ensure; only the lakeFS server is a fixture, and it stops answering
// object reads once armed. Before this, Ensure waited for the HTTP client
// timeout after cancel; now it returns context.Canceled promptly.
func TestEnsureCancelInterruptsHungLakeFSRead(t *testing.T) {
	var armed atomic.Bool
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	var releaseOnce sync.Once
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if !armed.Load() || r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/objects") {
			return false
		}
		entered <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
		return true
	})
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	id := kernel.RepositoryID("kr://cancel/lakefs")
	raw, err := snapshot.OpenLakeFS(id, fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(raw); err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(store).Require(id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := raw.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return &testfixture.StateEngine{}, nil })
	t.Cleanup(func() { _ = idx.Close() })

	armed.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := idx.Ensure(ctx, repo, commit); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("maintenance finished without reaching an object read: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("maintenance never reached a lakeFS object read")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled maintenance returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled maintenance stayed blocked in the lakeFS read")
	}
}
