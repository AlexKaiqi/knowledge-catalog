package lakefsimpl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// hangingLakeFS answers the commit probe, then never returns the object bytes,
// the way a stalled lakeFS or data plane does. entered signals the hung read.
func hangingLakeFS(t *testing.T, redirect bool) (endpoint Endpoint, entered chan struct{}) {
	t.Helper()
	entered = make(chan struct{}, 8)
	release := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/data":
			entered <- struct{}{}
			<-release
		case r.URL.Path == "/api/v1/repositories/system/commits/c1":
			_, _ = w.Write([]byte(`{"id":"c1"}`))
		case redirect:
			http.Redirect(w, r, server.URL+"/data", http.StatusFound)
		default:
			entered <- struct{}{}
			<-release
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	return Endpoint{API: server.URL + "/api/v1", Repository: "system"}, entered
}

// Canceling the bound context must end a read that is stuck on lakeFS or on
// the presigned data plane, within a bounded time and as context.Canceled,
// not as a missing commit or after the client timeout.
func TestBoundContextCancelsHungReads(t *testing.T) {
	for name, redirect := range map[string]bool{"api request": false, "presigned data plane": true} {
		t.Run(name, func(t *testing.T) {
			endpoint, entered := hangingLakeFS(t, redirect)
			client, err := newAPIClient(endpoint, "access:secret")
			if err != nil {
				t.Fatal(err)
			}
			repo := &Repository{id: "kr://cancel/read", ep: endpoint, client: client, repoState: &repoState{}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bound := repo.WithContext(ctx)
			done := make(chan error, 1)
			go func() { _, err := bound.ReadFile("README.md", "c1"); done <- err }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("read never reached the hung server")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("hung read returned %v, want context.Canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("canceled read stayed blocked until the client timeout")
			}
		})
	}
}

func TestBoundContextCancelsListingAndKeepsSharedCache(t *testing.T) {
	endpoint, entered := hangingLakeFS(t, false)
	client, err := newAPIClient(endpoint, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &Repository{id: "kr://cancel/list", ep: endpoint, client: client, repoState: &repoState{}}
	ctx, cancel := context.WithCancel(context.Background())
	bound := repo.WithContext(ctx)
	if !bound.HasCommit("c1") || !repo.HasCommit("c1") {
		t.Fatal("the commit probe failed before cancellation")
	}
	if _, ok := repo.verified["c1"]; !ok {
		t.Fatal("a bound view must share the unbound commit cache")
	}
	done := make(chan error, 1)
	go func() { _, err := bound.ListFiles("c1"); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("hung listing returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled listing stayed blocked")
	}
}

// An unbound repository keeps its former behavior: no context, no cancellation.
func TestUnboundRepositoryIsUnchanged(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"id":"c1"}`))
	}))
	defer server.Close()
	client, err := newAPIClient(Endpoint{API: server.URL + "/api/v1", Repository: "system"}, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &Repository{client: client, repoState: &repoState{}}
	var absent context.Context
	if repo.WithContext(absent).client != client {
		t.Fatal("a nil context must not produce a bound client")
	}
	if !repo.HasCommit("c1") || calls.Load() != 1 {
		t.Fatalf("unbound probe: calls=%d", calls.Load())
	}
}
