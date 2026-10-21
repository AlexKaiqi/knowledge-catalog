package home

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestSystemAuthorityContextCancelsEveryReadPort(t *testing.T) {
	operations := map[string]func(snapshot.Store, kernel.CommitID, kernel.CommitID) error{
		"head": func(s snapshot.Store, _, _ kernel.CommitID) error {
			_, err := s.Head(snapshot.DefaultRef)
			return err
		},
		"file": func(s snapshot.Store, _, commit kernel.CommitID) error {
			_, err := s.(snapshot.TreeReader).ReadFile("example.txt", commit)
			return err
		},
		"files": func(s snapshot.Store, _, commit kernel.CommitID) error {
			_, err := s.(snapshot.TreeReader).ListFiles(commit)
			return err
		},
		"directory": func(s snapshot.Store, _, commit kernel.CommitID) error {
			_, err := s.(snapshot.DirectoryReader).ReadDirectory(snapshot.DirectoryRequest{Commit: commit})
			return err
		},
		"history": func(s snapshot.Store, _, commit kernel.CommitID) error {
			_, err := s.(snapshot.HistoryStore).CommitHistory(commit, 10)
			return err
		},
		"changes": func(s snapshot.Store, base, commit kernel.CommitID) error {
			_, err := s.(snapshot.ChangeStore).ChangedPaths(base, commit)
			return err
		},
		"metadata": func(s snapshot.Store, _, _ kernel.CommitID) error {
			_, err := s.(nativeRepositoryMetadataReader).ReadRepositoryMetadata()
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			var block atomic.Bool
			entered, interrupted := make(chan struct{}, 1), make(chan struct{}, 1)
			cleanup := make(chan struct{})
			fake := testkit.NewLakeFSFakeWithInterceptor(t, func(_ http.ResponseWriter, r *http.Request) bool {
				if !block.Load() {
					return false
				}
				entered <- struct{}{}
				select {
				case <-r.Context().Done():
					interrupted <- struct{}{}
				case <-cleanup:
				}
				return true
			})
			t.Cleanup(func() { close(cleanup) })
			repoName := fake.NewRepo()
			source, err := snapshot.OpenLakeFS("kr://kc/system", fake.DSN(repoName), fake.Credential())
			if err != nil {
				t.Fatal(err)
			}
			base := testkit.MustHead(t, source, snapshot.DefaultRef)
			commit, err := fake.CommitFiles(repoName, base, []snapshot.TreeChange{{Path: "example.txt", Content: []byte("example")}})
			if err != nil {
				t.Fatal(err)
			}
			original, err := readOnlySystemAuthority(source)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bound := snapshot.BindStoreContext(original, ctx)
			requireReadOnlySystemView(t, bound, original)
			block.Store(true)
			result := make(chan error, 1)
			go func() { result <- operation(bound, base, commit) }()
			select {
			case <-entered:
			case err := <-result:
				t.Fatalf("read ended before reaching source: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("read never reached source")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("read returned %v, want context.Canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("System read stayed blocked after cancellation")
			}
			select {
			case <-interrupted:
			case <-time.After(5 * time.Second):
				t.Fatal("source request was not canceled")
			}
			block.Store(false)
			if err := operation(original, base, commit); err != nil {
				t.Fatalf("canceling a borrowed view changed the original: %v", err)
			}
		})
	}
}

func requireReadOnlySystemView(t *testing.T, bound, original snapshot.Store) {
	t.Helper()
	if bound.ID() != original.ID() || bound.(snapshot.StoreOrigin).Origin() != original.(snapshot.StoreOrigin).Origin() {
		t.Fatal("context binding changed System identity or origin")
	}
	if _, ok := bound.(snapshot.TreeStore); ok {
		t.Fatal("context binding exposed raw System writes")
	}
	if _, ok := bound.(metadataWrapperWriter); ok {
		t.Fatal("context binding exposed System metadata writes")
	}
	if kernel.CodeOf(bound.CreateRef("forbidden", "commit")) != kernel.ErrForbidden ||
		kernel.CodeOf(bound.Archive()) != kernel.ErrForbidden {
		t.Fatal("context binding permitted System mutation")
	}
	if _, err := bound.Merge("forbidden", "candidate", "expected"); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal("context binding permitted System merge")
	}
}

func TestSystemAuthorityContextPreservesAbsentMetadata(t *testing.T) {
	source := testkit.MakeTreeStore(t, "kr://kc/system")
	withoutMetadata := struct {
		snapshot.Store
		snapshot.TreeReader
		snapshot.DirectoryReader
		snapshot.HistoryStore
		snapshot.ChangeStore
	}{source, source.(snapshot.TreeReader), source.(snapshot.DirectoryReader), source.(snapshot.HistoryStore), source.(snapshot.ChangeStore)}
	original, err := readOnlySystemAuthority(withoutMetadata)
	if err != nil {
		t.Fatal(err)
	}
	bound := snapshot.BindStoreContext(original, context.Background())
	requireReadOnlySystemView(t, bound, original)
	if _, ok := bound.(nativeRepositoryMetadataReader); ok {
		t.Fatal("context binding invented optional metadata reads")
	}
	if snapshot.BindStoreContext(original, nil) != original {
		t.Fatal("nil context replaced the original view")
	}
}
