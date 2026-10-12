package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestDatasetConsoleDeepLinksAndAssetsUseServerBundle(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"})
	defer handler.(interface{ Close() error }).Close()
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}
	root := request("/ui/")
	if root.Code != 200 || root.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe or missing console shell: %d %v", root.Code, root.Header())
	}
	if policy := root.Header().Get("Content-Security-Policy"); policy != "" {
		t.Fatalf("console must preserve configured SSO, remote storage, images and PDF renderers: %s", policy)
	}
	// Both a fresh checkout and a built binary must serve its deep link. The
	// branch here selects the artifact under test, never skips an assertion.
	built := strings.Contains(root.Body.String(), "/ui/assets/")
	deep := request("/ui/catalogs/beta/datasets/shared/edit")
	if deep.Code != 200 || deep.Body.String() != root.Body.String() {
		t.Fatalf("deep link misses the console: %d", deep.Code)
	}
	if !built {
		if !strings.Contains(root.Body.String(), "snapshot/console/webapp/") || strings.Contains(root.Body.String(), "server/webapp/") || strings.Contains(root.Body.String(), "cli/webapp/") {
			t.Fatal("build hint points at the wrong embed owner")
		}
		return
	}
	scripts := regexp.MustCompile(`<script[^>]+src="([^"]+)"`).FindAllStringSubmatch(root.Body.String(), -1)
	if len(scripts) == 0 {
		t.Fatal("built shell has no script")
	}
	for _, script := range scripts {
		asset := request(script[1])
		if asset.Code != 200 || !strings.Contains(asset.Header().Get("Content-Type"), "javascript") || asset.Body.Len() == 0 {
			t.Fatalf("built script missing: %s (%d)", script[1], asset.Code)
		}
	}
	if request("/ui/assets/does-not-exist.js").Code != http.StatusNotFound {
		t.Fatal("missing script must not become a successful HTML fallback")
	}
}

func TestDatasetUIPageRoutesAndMissingStaticFiles(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"})
	defer handler.(interface{ Close() error }).Close()
	for _, path := range []string{
		"/ui/repositories/source.txt/objects",
		"/ui/auth/users/alice@example.com/credentials",
		"/ui/setup",
		"/ui/datasets",
		"/ui/catalogs/beta/datasets/readme.txt",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("page route must serve the console shell: %d %v", response.Code, response.Header())
			}
		})
	}
	for _, path := range []string{
		"/ui/assets/does-not-exist.js",
		"/ui/missing-logo.svg",
		"/ui/missing-robots.txt",
		"/ui/missing-manifest.webmanifest",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("missing static resource must return 404: %d", response.Code)
			}
		})
	}
}
