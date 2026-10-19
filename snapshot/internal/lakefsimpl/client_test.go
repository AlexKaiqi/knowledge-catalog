package lakefsimpl

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPresignedReadsReuseAPIConnection(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Redirect(w, r, server.URL+"/bytes", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("fixed bytes"))
	})
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client, err := newAPIClient(Endpoint{API: server.URL + "/api/v1", Repository: "system"}, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		content, err := client.getObject("fixed-commit", "README.md")
		if err != nil || string(content) != "fixed bytes" {
			t.Fatalf("presigned read: %q %v", content, err)
		}
	}
	if connections.Load() > 2 {
		t.Fatalf("redirect bodies prevented HTTP connection reuse: %d connections", connections.Load())
	}
}

// Staging and planning reads fan out over a bounded worker pool. The idle pool
// must hold that many connections per host, otherwise every round beyond two
// connections opens a fresh TCP connection and leaves the old one in TIME_WAIT.
func TestConcurrentRequestsReuseConnections(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		_, _ = w.Write([]byte("{}"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client, err := newAPIClient(Endpoint{API: server.URL + "/api/v1", Repository: "system"}, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	const workers, rounds = defaultStageConcurrency, 6
	for range rounds {
		var wg sync.WaitGroup
		wg.Add(workers)
		for range workers {
			go func() {
				defer wg.Done()
				if _, _, err := client.doJSON(http.MethodGet, "/repositories/system", nil, nil); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
	}
	if got := connections.Load(); got > workers+workers/4 {
		t.Fatalf("concurrent lakeFS requests churned connections: %d new connections for %d workers", got, workers)
	}
}

func uploadTestClient(t *testing.T, handler http.HandlerFunc) *apiClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := uploadBaseDelay
	uploadBaseDelay = time.Millisecond
	t.Cleanup(func() { uploadBaseDelay = previous })
	client, err := newAPIClient(Endpoint{API: server.URL + "/api/v1", Repository: "bulk"}, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestUploadObjectRetriesTransientServerErrors(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var bodies []string
	client := uploadTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, 16)
		n, _ := r.Body.Read(raw)
		mu.Lock()
		bodies = append(bodies, string(raw[:n]))
		mu.Unlock()
		if calls.Add(1) <= 3 {
			http.Error(w, `{"message":"cannot assign requested address"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	if err := client.uploadObject("wip", "a/b.yaml", []byte("payload")); err != nil {
		t.Fatalf("transient 5xx must be retried to success: %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("calls = %d, want 4", calls.Load())
	}
	for i, body := range bodies {
		if body != "payload" {
			t.Fatalf("attempt %d resent body %q, want the full payload", i, body)
		}
	}
}

func TestUploadObjectGivesUpAfterBoundedAttempts(t *testing.T) {
	var calls atomic.Int32
	client := uploadTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, `{"message":"backing store down"}`, http.StatusInternalServerError)
	})
	err := client.uploadObject("wip", "a.yaml", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "backing store down") {
		t.Fatalf("persistent failure must surface lakeFS body, got %v", err)
	}
	if calls.Load() != uploadAttempts {
		t.Fatalf("calls = %d, want %d", calls.Load(), uploadAttempts)
	}
}

func TestUploadObjectDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	client := uploadTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, `{"message":"bad path"}`, http.StatusBadRequest)
	})
	if err := client.uploadObject("wip", "a.yaml", []byte("x")); err == nil {
		t.Fatal("4xx must fail")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1: 4xx is not transient", calls.Load())
	}
}
