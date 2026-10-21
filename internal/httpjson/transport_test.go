package httpjson

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func countNewConnections(t *testing.T, transport http.RoundTripper, fanOut, rounds int) int64 {
	t.Helper()
	var opened atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client := &http.Client{Transport: transport}
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		wg.Add(fanOut)
		for i := 0; i < fanOut; i++ {
			go func() {
				defer wg.Done()
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}
		wg.Wait()
	}
	return opened.Load()
}

func TestPooledTransportReusesConnectionsAcrossFanOutRounds(t *testing.T) {
	const fanOut, rounds = 48, 6
	transport := PooledTransport(fanOut)
	defer transport.CloseIdleConnections()
	if opened := countNewConnections(t, transport, fanOut, rounds); opened > fanOut*5/4 {
		t.Fatalf("pooled transport opened %d connections for %d×%d requests; want at most %d", opened, fanOut, rounds, fanOut*5/4)
	}
	if got := PooledTransport(1).MaxIdleConnsPerHost; got < MinIdleConnsPerHost {
		t.Fatalf("idle pool floor = %d, want >= %d", got, MinIdleConnsPerHost)
	}
}
