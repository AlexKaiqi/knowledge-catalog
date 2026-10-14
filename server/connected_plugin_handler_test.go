package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	consumerplugin "kc/discovery/plugin"
)

func TestConnectedPluginHandlerColdConstructionAndExplanationDoNotProbe(t *testing.T) {
	f := newConnectedHandlerFixture(t)
	before := f.requests.Load()
	handler := f.handler(t)
	if got := f.requests.Load(); got != before {
		t.Errorf("cold handler construction contacted the connected authority: before=%d after=%d", before, got)
	}
	request := httptest.NewRequest(http.MethodGet, "/knowledge/v1/plugins/declarative-index", nil)
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
	activation, ok := handler.(interface{ Start() error })
	if !ok {
		t.Fatal("actual managed handler does not expose explicit runtime activation")
	}
	metadataBefore := f.metadataReads.Load()
	if err := activation.Start(); err != nil {
		t.Fatal(err)
	}
	if f.metadataReads.Load() != metadataBefore+1 {
		t.Fatal("explicit Start did not verify the native connected Repository plugin configuration once")
	}
	f.waitForStartedProjection(t, handler)
	response := fileGatewayRequest(t, handler, f.principal, "/operations/v1/projections:sync", map[string]any{
		"repository": f.canonical.Repository, "commit": f.canonical.Commit,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("actual configured plugin preparation failed: status=%d body=%s", response.Code, response.Body.String())
	}
	first := f.query(t, handler)
	if f.engineWrites.Load() == 0 {
		t.Fatal("actual fixed preparation did not publish to the real engine")
	}
	f.writeConfig(t, []consumerplugin.PluginSelection{})
	metadataBefore, writesBefore := f.metadataReads.Load(), f.engineWrites.Load()
	if err := activation.Start(); err != nil {
		t.Fatal(err)
	}
	if f.metadataReads.Load() != metadataBefore || f.engineWrites.Load() != writesBefore {
		t.Fatal("repeated Start reread Repository configuration or rebuilt the actual projection")
	}
	second := f.query(t, handler)
	if !reflect.DeepEqual(first.SearchView, second.SearchView) || !reflect.DeepEqual(first.Hits, second.Hits) {
		t.Fatalf("repeated Start changed the accepted runtime or fixed result: before=%#v after=%#v", first, second)
	}
	if err := handler.(interface{ Close() error }).Close(); err != nil {
		t.Fatal(err)
	}
	metadataBefore, writesBefore = f.metadataReads.Load(), f.engineWrites.Load()
	if err := activation.Start(); err == nil {
		t.Fatal("closed handler accepted runtime activation")
	}
	if f.metadataReads.Load() != metadataBefore || f.engineWrites.Load() != writesBefore {
		t.Fatal("rejected activation of a closed handler contacted configuration or rebuilt resources")
	}
}
