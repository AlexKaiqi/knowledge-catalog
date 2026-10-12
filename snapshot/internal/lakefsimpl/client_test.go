package lakefsimpl

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
