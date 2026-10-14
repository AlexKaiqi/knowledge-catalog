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
	snapshotclient "kc/snapshot/client"
)

func TestGrantUpdateClientKeepsExistingPUTContractAndObservation(t *testing.T) {
	calls := 0
	request := snapshotclient.GrantUpdateRequest{
		GrantRequest: snapshotclient.GrantRequest{Principal: "reader", Actions: []string{"file.read", "dataset.resolve"}, Dataset: "next"},
		Expected: snapshotclient.GrantExpected{
			GrantRequest: snapshotclient.GrantRequest{Principal: "reader", Actions: []string{"file.read"}, Repository: "repo-A", Dataset: "previous", Ref: "main", Object: "opaque/*", Aspect: "canonical"},
			ID:           "rule-A", SharedBy: "publisher",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut || r.URL.Path != "/admin/v1/grants/rule-A" || r.Header.Get("X-Kc-As") != "manager" || r.Header.Get("X-Kc-Request-Id") != "request-A" {
			t.Errorf("wrong update transport: %s %s %#v", r.Method, r.URL.Path, r.Header)
		}
		var actual snapshotclient.GrantUpdateRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&actual); err != nil || !reflect.DeepEqual(actual, request) {
			t.Errorf("update changed expected or replacement: %#v %v", actual, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"PRECONDITION_FAILED","message":"grant changed; reload before editing"}}`))
	}))
	defer server.Close()
	remote, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "manager"}}); err != nil {
		t.Fatal(err)
	}
	if err := remote.AdminService().UpdateGrant(context.Background(), "rule-A", request, snapshotclient.RequestOptions{RequestID: "request-A"}, nil); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || calls != 1 {
		t.Fatalf("update swallowed conflict, retried or refreshed the observation: calls=%d err=%v", calls, err)
	}
	request.Expected.ID = "other"
	if err := remote.AdminService().UpdateGrant(context.Background(), "rule-A", request, snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUsageInvalid || calls != 1 {
		t.Fatalf("mismatched identities reached transport: calls=%d err=%v", calls, err)
	}
}
