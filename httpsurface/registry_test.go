package httpsurface

import "testing"

func TestRegistryIsTheReviewedPublicHTTPSurface(t *testing.T) {
	got := Patterns()
	if len(got) != 65 {
		t.Fatalf("HTTP registry count changed from the reviewed 65 to %d", len(got))
	}
	seen := map[string]bool{}
	for _, pattern := range got {
		if seen[pattern] {
			t.Errorf("duplicate HTTP pattern %s", pattern)
		}
		seen[pattern] = true
	}
}
