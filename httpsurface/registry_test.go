package httpsurface

import (
	"strings"
	"testing"
)

func TestRegistryIsTheReviewedPublicHTTPSurface(t *testing.T) {
	got := Patterns()
	if len(got) != 92 {
		t.Fatalf("HTTP registry count changed from the reviewed 92 to %d", len(got))
	}
	seen := map[string]bool{}
	for _, pattern := range got {
		if seen[pattern] {
			t.Errorf("duplicate HTTP pattern %s", pattern)
		}
		if strings.Contains(pattern, "/catalog/") || strings.Contains(pattern, "{catalog}") {
			t.Errorf("HTTP exposes removed Catalog selector: %s", pattern)
		}
		if strings.Contains(strings.ToLower(pattern), "kset") {
			t.Errorf("public HTTP still uses kset alias: %s", pattern)
		}
		seen[pattern] = true
	}
	// Product onboarding adds these reviewed management and identity routes;
	// keeping the exact patterns guards against accidental replacement as well
	// as an unreviewed change in the total surface.
	for _, pattern := range []string{
		"GET /repositories/{repository}", "GET /assets/repository.js",
		"GET /console", "GET /assets/console.js",
		"GET /ui/",
		"GET /operations/v1/stores",
		"GET /snapshot/v1/connections", "GET /snapshot/v1/connections/{repository}",
		"POST /snapshot/v1/connections",
		"POST /snapshot/v1/connections:connect",
		"GET /snapshot/v1/connections/{repository}/connection",
		"POST /snapshot/v1/connections/{repository}/connection:check",
		"POST /snapshot/v1/connections/{repository}/connection:rotate",
		"GET /identity/v1/admission",
		"PUT /admin/v1/grants/{grant}",
		"GET /snapshot/v1/connections/{repository}/shares",
		"POST /snapshot/v1/connections/{repository}/shares",
		"DELETE /snapshot/v1/connections/{repository}/shares/{share}",
		"POST /identity/v1/token", "POST /identity/v1/authorize", "POST /identity/v1/authorize:poll",
	} {
		if !seen[pattern] {
			t.Errorf("reviewed product route missing: %s", pattern)
		}
	}
}
