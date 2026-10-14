package main

import "testing"

func TestAllowedRel(t *testing.T) {
	ok := []string{
		"README.md",
		"docs/product.html",
		"docs/reviewed/terminology.md",
		"docs/reviewed/terminology.md",
		"docs/reviewed/README.md",
		"docs/graph/README.md",
		"docs/assets/architecture.svg",
		"docs/assets/nested/architecture.png",
		"docs/assets/architecture.SVG",
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
		"architecture.svg",
		"docs/reviewed/architecture.svg",
		"docs/architecture.png",
		"docs/assets-other/architecture.svg",
		"docs/assets/architecture.gif",
		"docs/assets/architecture.webp",
		"docs/assets/architecture.svg.js",
		"docs/assets/../architecture.svg",
	}
	for _, p := range deny {
		if allowedRel(p) {
			t.Fatalf("want deny %q", p)
		}
	}
}
