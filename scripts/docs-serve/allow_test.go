package main

import "testing"

func TestAllowedRel(t *testing.T) {
	ok := []string{
		"README.md",
		"doc/README.md",
		"doc/concepts.md",
		"doc/design/object.md",
		"doc/engineering/test-catalog.md",
		"doc/product.html",
		"systemrepo/content/guides/first-publication/guide.md",
		"systemrepo/content/capabilities/read-object/definition.md",
		"doc/assets/architecture.svg",
		"doc/assets/nested/architecture.png",
		"doc/assets/architecture.SVG",
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
		"doc/../README.md",
		"doc/index.html",
		"doc/product.html.js",
		"systemrepo/content/page.html",
		"systemrepo/content/config.yaml",
		"systemrepo/content-other/guide.md",
		"systemrepo/README.md",
		"doc/design/x.okf",
		"docs/README.md",
		"/etc/passwd",
		"",
		"architecture.svg",
		"doc/design/architecture.svg",
		"doc/architecture.png",
		"doc/assets-other/architecture.svg",
		"doc/assets/architecture.gif",
		"doc/assets/architecture.webp",
		"doc/assets/architecture.svg.js",
		"doc/assets/../architecture.svg",
	}
	for _, p := range deny {
		if allowedRel(p) {
			t.Fatalf("want deny %q", p)
		}
	}
}
