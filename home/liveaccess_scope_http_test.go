package home

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

func TestDefaultLiveAccessEntrySeesLateManagedRepository(t *testing.T) {
	_, ws, _ := managedFixture(t)
	if err := ws.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Principal") != "alice" {
			t.Errorf("source request target or principal changed: %s %#v", r.URL, r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "observed", "basis": objectmodel.ObservationBasis{BindingGeneration: "generation", Consistency: objectmodel.ObservationLatestOnly, ObservedAt: "2026-10-16T03:00:00Z"}})
	}))
	defer origin.Close()
	id := kernel.RepositoryID("kr://managed/default-liveaccess")
	lookup := liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: id, Origin: origin.URL, Runtime: "http", Protocol: objectmodel.ProtocolResourceAccessV1, Operations: map[string]objectmodel.BindingOperation{"read": {Call: "value.read"}}}, Identity: kernel.IdentityContext{Principal: "alice"}}
	invoke := liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: id}, Runtime: "http", Protocol: objectmodel.ProtocolResourceAccessV1, Origin: origin.URL, Call: "value.read", Operation: "read", Identity: liveaccess.StateRuntimeIdentity{Principal: "alice"}}
	if _, err := ws.LiveAccess.LookupState(context.Background(), lookup); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unaccepted source reached default lookup: %v", err)
	}
	if _, err := ws.LiveAccess.AccessResource(context.Background(), invoke); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || calls.Load() != 0 {
		t.Fatalf("unaccepted source reached default operation: calls=%d err=%v", calls.Load(), err)
	}
	if _, err := ws.CreateManagedRepository(ManagedRepositoryRequest{RepositoryID: string(id), CommandID: "default-liveaccess", Principal: "alice"}, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.AttachRepository(id); err != nil {
		t.Fatal(err)
	}
	if observed, err := ws.LiveAccess.LookupState(context.Background(), lookup); err != nil || observed.Value != "observed" {
		t.Fatalf("default entry rejected accepted source: %#v %v", observed, err)
	}
	if _, err := ws.LiveAccess.AccessResource(context.Background(), invoke); err != nil || calls.Load() != 2 {
		t.Fatalf("default entry rejected accepted source operation: calls=%d err=%v", calls.Load(), err)
	}
}
