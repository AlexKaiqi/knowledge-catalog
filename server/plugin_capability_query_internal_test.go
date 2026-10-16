package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Malformed escapes are dropped by URL.Query(); the transport contract still
// refuses any query on plugin capability tasks.
func TestPluginCapabilityRejectsAnyQueryIncludingMalformed(t *testing.T) {
	facade := &httpFacade{}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/object/v1/plugins/declarative-index/capabilities"},
		{http.MethodPost, "/object/v1/plugins/declarative-index/declarations:trial"},
	} {
		for _, query := range []string{"?ignored=%zz", "?a=b", "?"} {
			t.Run(endpoint.method+query, func(t *testing.T) {
				r := httptest.NewRequest(endpoint.method, endpoint.path+query, strings.NewReader(`{"repository":"kr://trial/public"}`))
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, r)
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "USAGE_INVALID") || !strings.Contains(response.Body.String(), "query parameters") {
					t.Fatalf("query %q was not refused by the registered task: status=%d body=%s", query, response.Code, response.Body.String())
				}
			})
		}
	}
	r := httptest.NewRequest("GET", "/object/v1/plugins/declarative-index/capabilities", nil)
	if rejectPluginQuery(httptest.NewRecorder(), r) {
		t.Fatal("request without a query was refused")
	}
}
