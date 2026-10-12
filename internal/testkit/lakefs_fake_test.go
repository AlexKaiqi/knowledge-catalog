package testkit

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type closeSignalListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *closeSignalListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

func TestLakeFSFakeCloseAllowsInFlightRequestsToFinish(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	fake := &LakeFSFake{repos: map[string]*lakefsFakeRepo{}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fake.serveHTTP(w, r)
	}))
	listener := &closeSignalListener{Listener: server.Listener, closed: make(chan struct{})}
	server.Listener = listener
	fake.server = server
	server.Start()
	request, err := http.NewRequest(http.MethodGet, fake.Origin()+"/api/v1/repositories/"+fake.NewRepo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("access", "secret")
	read := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(request)
		if response != nil {
			response.Body.Close()
		}
		read <- err
	}()
	awaitFakeSignal(t, entered)
	closed := make(chan struct{})
	go func() { fake.Close(); close(closed) }()
	awaitFakeSignal(t, listener.closed)
	close(release)
	awaitFakeSignal(t, closed)
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	fake.Close()
}

func awaitFakeSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("lakeFS fixture shutdown blocked an in-flight request")
	}
}
