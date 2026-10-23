package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestConnectedPluginHandlerColdConstructionAndExplanationDoNotProbe(t *testing.T) {
	f := newConnectedHandlerFixture(t)
	before := f.requests.Load()
	handler := f.handler(t)
	if got := f.requests.Load(); got != before {
		t.Errorf("cold handler construction contacted the connected authority: before=%d after=%d", before, got)
	}
	request := httptest.NewRequest(http.MethodGet, "/object/v1/plugins/declarative-index", nil)
	request.Header.Set("X-Kc-As", f.principal)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("cold handler explanation failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if f.requests.Load() != before || f.engineWrites.Load() != 0 {
		t.Error("cold explanation activated the source or real projection engine")
	}
}

func TestConnectedPluginHandlerExplicitStartUsesOneFixedRuntime(t *testing.T) {
	f := newConnectedHandlerFixture(t)
	handler := f.handler(t)
	activation, ok := handler.(*ManagedHTTPHandler)
	if !ok {
		t.Fatal("actual managed handler does not expose explicit runtime activation")
	}
	var activations atomic.Int64
	activate := activation.activate
	activation.activate = func() error { activations.Add(1); return activate() }
	metadataBefore := f.metadataReads.Load()
	if err := activation.Start(); err != nil {
		t.Fatal(err)
	}
	if f.metadataReads.Load() != metadataBefore {
		t.Fatal("explicit Start read Repository metadata to choose plugins; the deployment installs them")
	}
	response := fileGatewayRequest(t, handler, f.principal, "/operations/v1/indexes:prepare", map[string]any{
		"dataset": "connected-articles", "revision": 1,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("actual configured plugin preparation failed: status=%d body=%s", response.Code, response.Body.String())
	}
	f.waitForStartedProjection(t, handler)
	first := f.query(t, handler)
	if f.engineWrites.Load() == 0 {
		t.Fatal("actual fixed preparation did not publish to the real engine")
	}
	// Background maintenance is allowed to finish unrelated work here.
	// Attribute repeated-Start I/O to activation, rather than elapsed counters.
	activationsBefore := activations.Load()
	if err := activation.Start(); err != nil {
		t.Fatal(err)
	}
	if activations.Load() != activationsBefore {
		t.Fatal("repeated Start reentered runtime activation")
	}
	second := f.query(t, handler)
	if !reflect.DeepEqual(first.SearchView, second.SearchView) || !reflect.DeepEqual(first.Hits, second.Hits) {
		t.Fatalf("repeated Start changed the accepted runtime or fixed result: before=%#v after=%#v", first, second)
	}
	if err := handler.(interface{ Close() error }).Close(); err != nil {
		t.Fatal(err)
	}
	metadataBefore = f.metadataReads.Load()
	writesBefore := f.engineWrites.Load()
	if err := activation.Start(); err == nil {
		t.Fatal("closed handler accepted runtime activation")
	}
	if f.metadataReads.Load() != metadataBefore || f.engineWrites.Load() != writesBefore {
		t.Fatal("rejected activation of a closed handler contacted configuration or rebuilt resources")
	}
}
