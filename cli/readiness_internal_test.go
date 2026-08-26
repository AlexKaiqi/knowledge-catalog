package cli

import (
	"testing"
	"time"
)

func TestReadinessCacheDoesNotRetainUnknownSurfaces(t *testing.T) {
	cache := newReadinessCache(t.TempDir(), time.Minute)
	for _, surface := range []string{"missing-a", "missing-b", "missing-c"} {
		if result := cache.surface(surface); result.ReasonCode != "SURFACE_UNKNOWN" {
			t.Fatalf("surface %q: %#v", surface, result)
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) != 0 || len(cache.inflight) != 0 {
		t.Fatalf("unknown surfaces polluted cache: entries=%d inflight=%d", len(cache.entries), len(cache.inflight))
	}
}
