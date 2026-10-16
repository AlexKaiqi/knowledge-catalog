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

func TestMaintenanceTransportPreservesFixedDatasetRevision(t *testing.T) {
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
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"basisCommit":"old","state":{"revision":"rev-2"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	o := RequestOptions{RequestID: "maintenance"}
	if err := remote.DescribeProjection(context.Background(), ProjectionRequest{Dataset: "chosen", Revision: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if err := remote.SyncProjection(context.Background(), ProjectionRequest{Dataset: "chosen", Revision: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/operations/v1/projections:describe,/operations/v1/projections:sync" {
		t.Fatal(paths)
	}
	if output.(map[string]any)["state"].(map[string]any)["revision"] != "rev-2" {
		t.Fatalf("actual StateSync result lost: %#v", output)
	}
}
