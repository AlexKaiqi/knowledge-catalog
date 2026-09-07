package httpsurface

import "testing"

func TestRegistryIsTheReviewedPublicHTTPSurface(t *testing.T) {
	got := Patterns()
	if len(got) != 66 {
		t.Fatalf("HTTP registry count changed from the reviewed 66 to %d", len(got))
	}
	seen := map[string]bool{}
	for _, pattern := range got {
		if seen[pattern] {
			t.Errorf("duplicate HTTP pattern %s", pattern)
		}
		seen[pattern] = true
	}
}
