package cli

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestConsolePageLoadsWithoutExposingAuthority(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"})
	defer handler.(interface{ Close() error }).Close()
	request := httptest.NewRequest(http.MethodGet, "/console", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	page := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(page, "观察台") || !strings.Contains(page, "成员仓") || !strings.Contains(page, "Snapshot 权威") || !strings.Contains(page, "检索投影") || !strings.Contains(page, "file.read") || !strings.Contains(page, "kc serve") || !strings.Contains(page, "关键字") || !strings.Contains(page, "search-fields") || !strings.Contains(page, "schema describe") || !strings.Contains(page, "不是重复实体") || strings.Contains(page, "Publish") {
		t.Fatalf("console has no usable read-only page: %d %s", response.Code, page)
	}
	if strings.Contains(response.Body.String(), "Book a demo") || strings.Contains(response.Body.String(), "Enterprise feature") {
		t.Fatal("console must not ship the lakeFS Datasets marketing gate")
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("console page must constrain executable content")
	}
	for _, private := range []string{".dolt", "/Users/", "client_secret", "managed.db"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("page shell exposes %s", private)
		}
	}
	scripts := regexp.MustCompile(`<script src="([^"]+)"`).FindAllStringSubmatch(response.Body.String(), -1)
	if len(scripts) != 1 || scripts[0][1] != "/assets/console.js" {
		t.Fatalf("console shell lacks its reviewed same-origin script: %v", scripts)
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, scripts[0][1], nil))
	script := asset.Body.String()
	if asset.Code != http.StatusOK || asset.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || script != consolePageJS {
		t.Fatalf("console script unavailable or changed in transport: status=%d type=%s", asset.Code, asset.Header().Get("Content-Type"))
	}
	if !strings.Contains(script, "/knowledge/v1/schemas:describe") || !strings.Contains(script, "/knowledge/v1/objects:read") || !strings.Contains(script, "/knowledge/v1/search") {
		t.Fatal("console script must exercise schema describe, READ and SEARCH")
	}
	if !strings.Contains(script, "runSchemaDescribe") {
		t.Fatal("console script must run schema describe from the schema list")
	}
	if asset.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("console script must prohibit content sniffing")
	}
}
