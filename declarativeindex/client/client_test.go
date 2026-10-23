package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(context.Context, string, *http.Request) (*http.Response, error)

func (f transportFunc) Do(ctx context.Context, audience string, r *http.Request) (*http.Response, error) {
	return f(ctx, audience, r)
}

// The index family keeps one maintenance scope: a named Dataset and an
// optional revision. A readiness answer is a normal result, so the transport
// must not widen it into repository/commit/ref coordinates.
func TestIndexTransportPreservesFixedDatasetRevision(t *testing.T) {
	paths := []string{}
	remote, err := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, audience string, r *http.Request) (*http.Response, error) {
		if audience != "https://example.test" || r.Method != "POST" || r.Header.Get("X-Kc-Request-Id") != "maintenance" {
			t.Fatalf("authenticated transport lost: audience=%s request=%#v", audience, r)
		}
		paths = append(paths, r.URL.Path)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["dataset"] != "chosen" || body["revision"] != float64(2) || body["repository"] != nil || body["commit"] != nil || body["ref"] != nil {
			t.Fatalf("Dataset maintenance scope changed: %#v", body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"dataset":"chosen","revision":2,"ready":false}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	o := RequestOptions{RequestID: "maintenance"}
	if err := remote.IndexReady(context.Background(), IndexRequest{Dataset: "chosen", Revision: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if err := remote.IndexPrepare(context.Background(), IndexRequest{Dataset: "chosen", Revision: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if err := remote.IndexDescribe(context.Background(), IndexRequest{Dataset: "chosen", Revision: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/operations/v1/indexes:ready,/operations/v1/indexes:prepare,/operations/v1/indexes:describe" {
		t.Fatal(paths)
	}
	if output.(map[string]any)["ready"] != false {
		t.Fatalf("not-ready answer must survive as a result, not an error: %#v", output)
	}
}

// Catch-up progress names no Dataset: the empty body is the whole request.
func TestIndexStatusTransportsEmptyScope(t *testing.T) {
	bodies := []string{}
	remote, err := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, r.URL.Path+":"+strings.TrimSpace(string(raw)))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`[{"repository":"kr://r","done":3,"total":9}]`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	if err := remote.IndexStatus(context.Background(), RequestOptions{RequestID: "maintenance"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Join(bodies, ",") != "/operations/v1/indexes:status:{}" {
		t.Fatal(bodies)
	}
	entries, ok := output.([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("catch-up progress lost: %#v", output)
	}
	if entries[0].(map[string]any)["total"] != float64(9) {
		t.Fatalf("progress denominator lost: %#v", entries[0])
	}
}
