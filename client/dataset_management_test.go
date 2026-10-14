package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/client"
	"kc/kernel"
	snapshotclient "kc/snapshot/client"
)

func datasetManagementClient(t *testing.T, handler http.Handler) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "reader"}}); err != nil {
		t.Fatal(err)
	}
	return kc
}

func TestDatasetShowPreservesQueryAndDefaultEntry(t *testing.T) {
	calls := 0
	kc := datasetManagementClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/snapshot/v1/datasets/delivery%20group" || r.Header.Get("X-Kc-As") != "reader" {
			t.Errorf("detail request: %s %s principal=%q", r.Method, r.URL.RequestURI(), r.Header.Get("X-Kc-As"))
		}
		query := r.URL.Query()
		if calls == 1 && (query.Get("revision") != "1" || query.Get("limit") != "5" || query.Get("continuation") != "basis+/=") {
			t.Errorf("detail query lost: %v", query)
		}
		if calls == 2 && len(query) != 0 {
			t.Errorf("legacy entry acquired selectors: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"delivery group","revision":1,"metadata":{"description":"selected version"}}`))
	}))
	revision := 1
	var output map[string]any
	if err := kc.SnapshotService().ShowDataset(context.Background(), "delivery group", snapshotclient.DatasetShowOptions{Revision: &revision, Limit: 5, Continuation: "basis+/="}, snapshotclient.RequestOptions{}, &output); err != nil {
		t.Fatal(err)
	}
	if output["metadata"].(map[string]any)["description"] != "selected version" {
		t.Fatalf("selected metadata lost: %#v", output)
	}
	if err := kc.SnapshotService().Dataset(context.Background(), "delivery group", snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int{0, -1} {
		if err := kc.SnapshotService().ShowDataset(context.Background(), "delivery group", snapshotclient.DatasetShowOptions{Revision: &revision}, snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid detail revision %d: %v", revision, err)
		}
	}
	if calls != 2 {
		t.Fatalf("invalid selectors sent requests: %d", calls)
	}
}

func TestDatasetResolveRevisionIsExplicitAndExclusive(t *testing.T) {
	calls := 0
	kc := datasetManagementClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/snapshot/v1/datasets/delivery/resolve" || request["revision"] != float64(1) || request["pin"] != nil {
			t.Errorf("resolve selector changed: %s %s %#v", r.Method, r.URL.Path, request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"setId":"delivery","revision":1}`))
	}))
	revision := 1
	if err := kc.SnapshotService().ResolveDataset(context.Background(), "delivery", snapshotclient.DatasetResolveRequest{Revision: &revision}, snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := kc.SnapshotService().CheckDataset(context.Background(), "delivery", snapshotclient.DatasetResolveRequest{Revision: &revision}, snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("check accepted a new revision selector: %v", err)
	}
	for _, request := range []snapshotclient.DatasetResolveRequest{
		{Revision: new(int)},
		{Revision: &revision, Pin: json.RawMessage(`{}`)},
	} {
		if err := kc.SnapshotService().ResolveDataset(context.Background(), "delivery", request, snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid resolve selector: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid resolve selectors sent requests: %d", calls)
	}
}
