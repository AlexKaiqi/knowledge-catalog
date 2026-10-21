package home

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
)

type connectionContextFixture struct {
	source     *connectedSource
	record     connectionRecord
	credential atomic.Value
	blocked    atomic.Bool
	entered    chan struct{}
}

func newConnectionContextFixture(t *testing.T, blockPath string) *connectionContextFixture {
	t.Helper()
	f := &connectionContextFixture{entered: make(chan struct{}, 1)}
	f.credential.Store("access:original-secret")
	f.blocked.Store(blockPath != "")
	release := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, secret, ok := r.BasicAuth()
		if !ok || user+":"+secret != f.credential.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.blocked.Load() && r.URL.Path == blockPath {
			f.entered <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		switch r.URL.Path {
		case "/api/v1/repositories/source":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case "/api/v1/repositories/source/branches/main":
			_, _ = w.Write([]byte(`{"commit_id":"c1"}`))
		case "/api/v1/repositories/source/commits/c0":
			_, _ = w.Write([]byte(`{"id":"c0"}`))
		case "/api/v1/repositories/source/refs/c0/objects":
			_, _ = w.Write([]byte("source content"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { close(release); provider.Close() })
	dir := t.TempDir()
	if err := createConnectionLedger(filepath.Join(dir, connectionLedgerFile)); err != nil {
		t.Fatal(err)
	}
	f.record = connectionRecord{
		AllocationID: strings.Repeat("a", 32), Owner: "alice",
		Binding:   RepositoryBinding{ID: "kr://cancel/source", Driver: "lakefs", DSN: provider.URL + "/source"},
		BackendID: "source", InitialHead: "c0", Phase: "READY",
		Actions: []string{"repository.connections.manage", "object.read"},
	}
	if err := saveConnection(dir, &f.record, f.credential.Load().(string), 0); err != nil {
		t.Fatal(err)
	}
	f.source = &connectedSource{dir: dir, id: kernel.RepositoryID(f.record.Binding.ID)}
	return f
}

// A connected source reopens its authority on every read. Cancellation must
// therefore cover opening and the fixed-identity proof as well as the read.
func TestConnectedSourceContextCancelsEveryNetworkStage(t *testing.T) {
	for name, path := range map[string]string{
		"opening authority": "/api/v1/repositories/source",
		"opening head":      "/api/v1/repositories/source/branches/main",
		"initial commit":    "/api/v1/repositories/source/commits/c0",
		"object content":    "/api/v1/repositories/source/refs/c0/objects",
	} {
		t.Run(name, func(t *testing.T) {
			f := newConnectionContextFixture(t, path)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bound := snapshot.BindStoreContext(f.source, ctx)
			tree, ok := snapshot.TreeReaderOf(bound)
			if !ok || bound.ID() != f.source.ID() {
				t.Fatal("context binding lost source identity or file reads")
			}
			done := make(chan error, 1)
			go func() { _, err := tree.ReadFile("README.md", "c0"); done <- err }()
			select {
			case <-f.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("read never reached the blocked authority stage")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled stage returned %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("canceled source read remained blocked")
			}
			f.blocked.Store(false)
			body, err := f.source.ReadFile("README.md", "c0")
			if err != nil || string(body) != "source content" {
				t.Fatalf("canceling a view changed the shared source: %q %v", body, err)
			}
		})
	}
}

func TestConnectedSourceBoundViewUsesRotatedCredential(t *testing.T) {
	f := newConnectionContextFixture(t, "")
	bound := snapshot.BindStoreContext(f.source, context.Background())
	tree, ok := snapshot.TreeReaderOf(bound)
	if !ok {
		t.Fatal("bound source lost file reads")
	}
	if _, err := tree.ReadFile("README.md", "c0"); err != nil {
		t.Fatal(err)
	}
	f.credential.Store("access:rotated-secret")
	if err := saveConnection(f.source.dir, &f.record, f.credential.Load().(string), f.record.Revision); err != nil {
		t.Fatal(err)
	}
	body, err := tree.ReadFile("README.md", "c0")
	if err != nil || string(body) != "source content" {
		t.Fatalf("bound view retained the old credential: %q %v", body, err)
	}
}

func TestConnectionFailurePreservesCancellationWithoutPrivateDetails(t *testing.T) {
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			fault := fmt.Errorf("https://private-source credential=private-secret: %w", sentinel)
			if got := connectionFailure(fault); got != sentinel {
				t.Fatalf("connection failure lost its context sentinel or retained private details: %v", got)
			}
		})
	}
}
