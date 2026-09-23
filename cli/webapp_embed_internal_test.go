package cli

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TestDatasetUILoadsBuildHintWithoutExposingAuthority owns the "GET /ui/"
// evidence entry: the Dataset web console route serves pages only, never
// authority. The route answers with the built console shell when webui/ has
// been built into cli/webapp, and with the build hint page otherwise; either
// way the page must carry no credentials or home layout.
func TestDatasetUILoadsBuildHintWithoutExposingAuthority(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"})
	defer handler.(interface{ Close() error }).Close()

	redirect := httptest.NewRecorder()
	handler.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/ui", nil))
	// ServeMux answers the slash-less subtree root with its 307 redirect.
	if redirect.Code != http.StatusTemporaryRedirect || redirect.Header().Get("Location") != "/ui/" {
		t.Fatalf("/ui must redirect to /ui/: %d %s", redirect.Code, redirect.Header().Get("Location"))
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	page := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("dataset UI route failed: %d %s", response.Code, page)
	}
	built := strings.Contains(page, "/ui/assets/")
	hint := strings.Contains(page, "webui/")
	if !built && !hint {
		t.Fatalf("dataset UI route serves neither the built console nor the build hint: %s", page)
	}
	if strings.Contains(page, "managed.db") || strings.Contains(page, "client_secret") || strings.Contains(page, "/Users/") {
		t.Fatalf("dataset UI page exposes private material: %s", page)
	}
	if hint && strings.Contains(response.Header().Get("Cache-Control"), "max-age") {
		t.Fatalf("dataset UI build hint must not be cached: %s", response.Header().Get("Cache-Control"))
	}
	if hint && strings.Contains(page, "\"code\"") && strings.Contains(page, "\"message\"") {
		// The hint is static content; it must never look like a FaultJSON route.
		t.Fatalf("dataset UI page must not answer like a protocol route: %s", page)
	}
	if built {
		// Same-origin scripts only: the shell may reference /ui/assets and the
		// authorized typed routes below, never a third-party origin.
		for _, script := range scriptSrcPattern.FindAllStringSubmatch(page, -1) {
			if !strings.HasPrefix(script[1], "/ui/") {
				t.Fatalf("dataset UI shell loads a non-same-origin script %q", script[1])
			}
		}
	}
}

var scriptSrcPattern = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
