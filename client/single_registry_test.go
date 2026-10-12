package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/client"
)

func TestSingleRegistryInventoryHasNoCatalogCoordinate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RequestURI() != "/snapshot/v1/inventory" {
			t.Errorf("inventory request = %s %s", r.Method, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repositories":[],"datasets":[]}`))
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	var out any
	if err := kc.SnapshotService().Show(context.Background(), client.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryAuditPreservesBoundedPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/snapshot/v1/audit" || r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("continuation") != "basis+/=" {
			t.Errorf("registry audit request = %s %s", r.Method, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entries":[]}`))
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	if err := kc.SnapshotService().Audit(context.Background(), 5, "basis+/=", client.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
}
