package server

import (
	"reflect"
	"testing"
)

// TestRoutesMatchGoldenManifest reconciles the in-package closed set with the
// exported golden manifest (routes_golden.txt): the manifest is what external
// scripts (validation-inventory) and tests (the CLI evidence inventory) read,
// so a route change on either side without the other fails here. The manifest
// and the literal table live in the same package and change together.
func TestRoutesMatchGoldenManifest(t *testing.T) {
	declared := map[string]bool{}
	for _, r := range Routes() {
		declared[r.Method+" "+r.Pattern] = true
	}
	golden := GoldenRoutes()
	if len(golden) == 0 {
		t.Fatal("golden manifest is empty; the closed set must have entries")
	}
	for _, entry := range golden {
		if !declared[entry] {
			t.Errorf("golden manifest entry %q is absent from routes.go; update both in the same change", entry)
		}
	}
	for entry := range declared {
		found := false
		for _, goldenEntry := range golden {
			if goldenEntry == entry {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("routes.go entry %q is absent from the golden manifest; update both in the same change", entry)
		}
	}
}

// TestGoldenManifestIsSortedAndUnique keeps the exported file in stable lexical
// order so external readers can compare element-wise without re-sorting.
func TestGoldenManifestIsSortedAndUnique(t *testing.T) {
	golden := GoldenRoutes()
	for i := 1; i < len(golden); i++ {
		if golden[i-1] >= golden[i] {
			t.Errorf("golden manifest must be strictly sorted and unique: %q then %q", golden[i-1], golden[i])
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

// TestGoldenRoutesExportedShape freezes the exported accessor's result so an
// accidental change to the embedding or parsing is caught at the owner.
func TestGoldenRoutesExportedShape(t *testing.T) {
	golden := GoldenRoutes()
	if len(golden) != len(routes) {
		t.Fatalf("golden manifest has %d entries, routes.go has %d", len(golden), len(routes))
	}
	if !reflect.DeepEqual(golden, mustSortedCopy(golden)) {
		t.Fatal("GoldenRoutes must return lexically sorted entries")
	}
}

func mustSortedCopy(values []string) []string {
	out := append([]string{}, values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
