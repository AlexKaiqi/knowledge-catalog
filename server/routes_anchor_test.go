package server

import (
	"kc/httpsurface"
	"testing"
)

// TestRoutesMatchHTTPSurfaceRegistry reconciles the implementation-side closed
// set against the formal HTTP surface contract: every route the server declares
// must appear in httpsurface.Patterns(), and every formal pattern must be
// declared here. A change on either side without the other fails this test, so
// the two faces of the access surface cannot drift apart.
func TestRoutesMatchHTTPSurfaceRegistry(t *testing.T) {
	formal := map[string]bool{}
	for _, pattern := range httpsurface.Patterns() {
		formal[pattern] = true
	}
	declared := map[string]bool{}
	for _, r := range Routes() {
		key := r.Method + " " + r.Pattern
		if !formal[key] {
			t.Errorf("server route %q is absent from httpsurface.Patterns(); update the formal registry in the same change", key)
		}
		declared[key] = true
	}
	for key := range formal {
		if !declared[key] {
			t.Errorf("formal surface entry %q is not declared in server routes; implement or retire it on both sides in the same change", key)
		}
	}
}

// TestRoutesHaveNoDuplicates keeps the closed set a set: registering the same
// method+pattern twice would panic when the mux is built, and this test names
// the duplicate at the contract layer instead.
func TestRoutesHaveNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range routes {
		key := r.Method + " " + r.Pattern
		if seen[key] {
			t.Errorf("duplicate route %q", key)
		}
		seen[key] = true
	}
	if len(seen) != len(routes) {
		t.Errorf("routes contains %d entries but only %d unique method+pattern pairs", len(routes), len(seen))
	}
}

// TestRoutesStaySorted keeps the literal table in stable order so contract
// diffs read as one line per route change.
func TestRoutesStaySorted(t *testing.T) {
	ordered := Routes()
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].Pattern > ordered[i].Pattern ||
			(ordered[i-1].Pattern == ordered[i].Pattern && ordered[i-1].Method > ordered[i].Method) {
			t.Errorf("routes literal is out of order at %q; keep the table sorted by (pattern, method)", ordered[i].Method+" "+ordered[i].Pattern)
		}
	}
}
