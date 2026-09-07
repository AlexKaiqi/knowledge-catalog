package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/client"
	"kc/kernel"
)

func TestManagedRepositoryCreatePreservesTypedRequestAndCorrelation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/catalog/v1/catalogs/catalog-A/repositories:create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Kc-As") != "agent:creator" || r.Header.Get("X-Kc-Request-Id") != "request-A" {
			t.Errorf("missing request identity or correlation: %#v", r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if want := map[string]any{"repository": "repo-A", "commandId": "create-A"}; !reflect.DeepEqual(body, want) {
			t.Errorf("typed request = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repositoryId":"repo-A","commandId":"create-A","status":"REPLAYED"}`))
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "agent:creator"}}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	err = kc.CatalogService().CreateRepository(context.Background(), "catalog-A", client.RepositoryCreateRequest{Repository: "repo-A", CommandID: "create-A"}, client.RequestOptions{RequestID: "request-A"}, &result)
	if err != nil || result["status"] != "REPLAYED" || result["repositoryId"] != "repo-A" {
		t.Fatalf("create result=%#v err=%v", result, err)
	}
}

func TestManagedRepositoryCreateRequiresExplicitCoordinatesBeforeTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusTeapot) }))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		catalog string
		request client.RepositoryCreateRequest
	}{
		{"", client.RepositoryCreateRequest{Repository: "repo-A", CommandID: "create-A"}},
		{"catalog-A", client.RepositoryCreateRequest{CommandID: "create-A"}},
		{"catalog-A", client.RepositoryCreateRequest{Repository: "repo-A"}},
	} {
		if err := kc.CatalogService().CreateRepository(context.Background(), input.catalog, input.request, client.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("missing input reached transport: %v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("incomplete request made %d network calls", calls)
	}
}
