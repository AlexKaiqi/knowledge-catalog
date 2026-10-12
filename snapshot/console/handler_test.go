package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDatasetConsoleHandlerCanBeMountedWithoutKCServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /ui/", Handler())
	page := httptest.NewRecorder()
	mux.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("independent console shell missing: %d %v", page.Code, page.Header())
	}
	deep := httptest.NewRecorder()
	mux.ServeHTTP(deep, httptest.NewRequest(http.MethodGet, "/ui/auth/grants", nil))
	if deep.Code != http.StatusOK || deep.Body.String() != page.Body.String() {
		t.Fatalf("independent mount lost the original console deep link: %d", deep.Code)
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/ui/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatal("independent console mount must preserve missing asset 404")
	}
}

func TestDatasetUIRoutingIndependentOfBuildState(t *testing.T) {
	for _, built := range []bool{false, true} {
		root := fstest.MapFS{".gitkeep": {}}
		if built {
			root["index.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>Console</title>")}
			root["assets/app.js"] = &fstest.MapFile{Data: []byte("console.log('app');")}
		}
		for path, want := range map[string]int{
			"/ui/":                                  http.StatusOK,
			"/ui/repositories/source.txt/objects":   http.StatusOK,
			"/ui/datasets/readme.txt":               http.StatusOK,
			"/ui/datasets/actions/create":           http.StatusOK,
			"/ui/catalogs/beta/datasets/readme.txt": http.StatusNotFound,
			"/ui/assets/missing.js":                 http.StatusNotFound,
			"/ui/assets/":                           http.StatusNotFound,
			"/ui/missing-logo.svg":                  http.StatusNotFound,
		} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			serveWebapp(response, request, root, built)
			if response.Code != want {
				t.Fatalf("built=%t path=%s: want %d, got %d", built, path, want, response.Code)
			}
			if request.URL.Path != path {
				t.Fatalf("console fallback modified the request path: %s", request.URL.Path)
			}
		}
	}
}
