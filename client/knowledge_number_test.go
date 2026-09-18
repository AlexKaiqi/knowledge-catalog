package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/client"
	"kc/knowledge"
)

func TestKnowledgeClientReadPreservesLargeIntegerValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/knowledge/v1/objects:read" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repository":"kr://acme/public/core","commit":"fixed","address":{"kind":"Entity","objectId":"sample/A"},"value":{"first":9007199254740992,"second":9007199254740993,"max":9223372036854775807}}`))
	}))
	t.Cleanup(server.Close)
	c, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
		if _, err := c.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "reader"}}); err != nil {
		t.Fatal(err)
	}
	var value knowledge.KnowledgeValue
	if err := c.KnowledgeService().Read(context.Background(), client.KnowledgeReadRequest{Repository: "kr://acme/public/core", Commit: "fixed", Object: "sample/A"}, client.RequestOptions{}, &value); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value.Value)
	if err != nil || string(encoded) != `{"first":9007199254740992,"max":9223372036854775807,"second":9007199254740993}` {
		t.Fatalf("Client changed authoritative integer values: %s, %v", encoded, err)
	}
}
