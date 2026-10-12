package service_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSingleRegistryRoutesRequireNoCatalogSelection(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	datasetHTTPCall(t, h, http.MethodGet, "/snapshot/v1/datasets", nil, http.StatusOK)
	for _, path := range []string{"/catalog/v1/catalogs", "/catalog/v1/catalogs/A/datasets"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Kc-As", "alice")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("retired Catalog route %s returned %d", path, response.Code)
		}
	}
}
