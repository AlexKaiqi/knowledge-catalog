package httpsurface

import "testing"

func TestRegistryIsTheReviewedPublicHTTPSurface(t *testing.T) {
	got := Patterns()
	if len(got) != 83 {
		t.Fatalf("HTTP registry count changed from the reviewed 83 to %d", len(got))
	}
	seen := map[string]bool{}
	for _, pattern := range got {
		if seen[pattern] {
			t.Errorf("duplicate HTTP pattern %s", pattern)
		}
		seen[pattern] = true
	}
	// Product onboarding adds these reviewed management and identity routes;
	// keeping the exact patterns guards against accidental replacement as well
	// as an unreviewed change in the total surface.
	for _, pattern := range []string{
		"GET /repositories/{repository}", "GET /assets/repository.js",
		"GET /catalog/v1/repositories", "GET /catalog/v1/repositories/{repository}",
		"POST /catalog/v1/repositories",
		"POST /catalog/v1/catalogs/{catalog}/repositories:connect",
		"GET /catalog/v1/repositories/{repository}/connection",
		"POST /catalog/v1/repositories/{repository}/connection:check",
		"POST /catalog/v1/repositories/{repository}/connection:rotate",
		"GET /identity/v1/admission", "POST /identity/v1/admission",
		"GET /catalog/v1/repositories/{repository}/shares",
		"POST /catalog/v1/repositories/{repository}/shares",
		"DELETE /catalog/v1/repositories/{repository}/shares/{share}",
		"POST /identity/v1/token", "POST /identity/v1/authorize", "POST /identity/v1/authorize:poll",
	} {
		if !seen[pattern] {
			t.Errorf("reviewed product route missing: %s", pattern)
		}
	}
}
