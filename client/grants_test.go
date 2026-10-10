package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/client"
	"kc/kernel"
)

func TestGrantClientUsesExistingInventoryCoordinates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/admin/v1/grants" || r.URL.Query().Get("repository") != "kr://acme/one" || r.URL.Query().Get("principal") != "agent:reader" || r.URL.Query().Get("object") != "data/*" || r.URL.Query().Get("action") != "knowledge.read" {
			t.Errorf("typed inventory query lost coordinates: %s %s", r.Method, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rules":[]}`))
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	query := client.GrantRequest{Principal: "agent:reader", Repository: "kr://acme/one", Object: "data/*", Actions: []string{"knowledge.read"}}
	var output map[string]any
	if err := kc.AdminService().GrantsMatching(context.Background(), query, client.RequestOptions{}, &output); err != nil || output["rules"] == nil {
		t.Fatalf("inventory: %#v %v", output, err)
	}
	query.Actions = append(query.Actions, "writer.commit")
	if err := kc.AdminService().GrantsMatching(context.Background(), query, client.RequestOptions{}, &output); kernel.CodeOf(err) != kernel.ErrUsageInvalid || requests != 1 {
		t.Fatalf("ambiguous action filter was sent: requests=%d err=%v", requests, err)
	}
}
