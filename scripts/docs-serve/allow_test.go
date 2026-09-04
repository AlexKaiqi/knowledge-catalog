package main

import "testing"

func TestAllowedRel(t *testing.T) {
	ok := []string{
		"README.md",
		"docs/product.html",
		"docs/TERMINOLOGY.md",
		"docs/graph/README.md",
	}
	for _, p := range ok {
		if !allowedRel(p) {
			t.Fatalf("want allow %q", p)
		}
	}
	deny := []string{
		"../README.md",
		"cmd/kc/main.go",
		".env",
		"docs/../README.md",
		"docs/graph/documents/x.okf",
		"/etc/passwd",
		"",
	}
	for _, p := range deny {
		if allowedRel(p) {
			t.Fatalf("want deny %q", p)
		}
	}
}
