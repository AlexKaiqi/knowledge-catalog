package cli

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestRepositoryManagementPageLoadsWithoutExposingAuthority(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"})
	defer handler.(interface{ Close() error }).Close()
	request := httptest.NewRequest(http.MethodGet, "/repositories/kr:%2F%2Fkaiqidong%2Frepo-one", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "观察台") || !strings.Contains(response.Body.String(), "/console") {
		t.Fatalf("management address has no usable page: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("management page must constrain executable content")
	}
	for _, private := range []string{".dolt", "/Users/", "client_secret", "managed.db"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("page shell exposes %s", private)
		}
	}
	// Follow the actual script URL emitted in the public shell. A separately
	// compiled VM test is not proof that the HTTP server serves its asset.
	scripts := regexp.MustCompile(`<script src="([^"]+)"`).FindAllStringSubmatch(response.Body.String(), -1)
	if len(scripts) != 1 || scripts[0][1] != "/assets/repository.js" {
		t.Fatalf("management shell lacks its reviewed same-origin script: %v", scripts)
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, scripts[0][1], nil))
	if asset.Code != http.StatusOK || asset.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || asset.Body.String() != repositoryPageJS {
		t.Fatalf("management script unavailable or changed in transport: status=%d type=%s", asset.Code, asset.Header().Get("Content-Type"))
	}
	if asset.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("management script must prohibit content sniffing")
	}
}
