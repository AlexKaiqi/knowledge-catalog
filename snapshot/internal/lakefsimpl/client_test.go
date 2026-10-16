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
