package auth

import (
	"net/http"
	"testing"

	"kc/internal/httpjson"
)

// The default client must not fall back to Go's two idle connections per
// host: CLI preflights fan out dozens of concurrent requests to one Server.
func TestDefaultHTTPClientPoolsConnectionsForFanOut(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok || transport.MaxIdleConnsPerHost < httpjson.MinIdleConnsPerHost {
		t.Fatalf("default client transport does not pool connections: %#v", c.httpClient.Transport)
	}
}
