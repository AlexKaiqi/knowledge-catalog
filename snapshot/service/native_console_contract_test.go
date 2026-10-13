package service

import (
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Check the retained caller rather than duplicating the gateway's route table.
// A native operation used by the pinned console must have a reviewed mapping.
func TestNativeRoutesCoverVendoredConsoleRepositoryCalls(t *testing.T) {
	raw, err := os.ReadFile("../console/webui/src/lib/api/index.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	starts := regexp.MustCompile("apiRequest\\(([`'\"])(/repositories[^\\n]*)").FindAllStringSubmatchIndex(source, -1)
	parameter := regexp.MustCompile(`\$\{[^}]+\}`)
	methodPattern := regexp.MustCompile(`method:\s*['"]([A-Z]+)['"]`)
	checked := 0
	for _, start := range starts {
		quote := source[start[2]]
		suffix := source[start[4]:]
		end := strings.IndexByte(suffix, quote)
		if end < 0 {
			t.Fatal("unterminated original API URL")
		}
		path := strings.Split(parameter.ReplaceAllString(suffix[:end], "sample"), "?")[0]
		options := suffix[end+1:]
		if end := strings.IndexByte(options, ';'); end >= 0 {
			options = options[:end]
		}
		method := http.MethodGet
		if match := methodPattern.FindStringSubmatch(options); match != nil {
			method = match[1]
		}
		request := &http.Request{Method: method, URL: &url.URL{Path: "/api/v1" + path}}
		if _, _, ok := matchNativeRoute(request); !ok {
			t.Errorf("original console operation has no mapping: %s %s", method, path)
		}
		checked++
	}
	if checked < 45 {
		t.Fatalf("original caller inspection unexpectedly shrank to %d operations", checked)
	}
}
