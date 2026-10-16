package main

import (
	"path/filepath"
	"strings"
)

// allowedRel is the closed set docs-serve serves: documentation Markdown,
// published system Markdown, the product book, and documentation images.
func allowedRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return false
	}
	if rel == "README.md" || rel == "doc/product.html" {
		return true
	}
	if strings.HasPrefix(rel, "systemrepo/content/") {
		return strings.EqualFold(filepath.Ext(rel), ".md")
	}
	if !strings.HasPrefix(rel, "doc/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md":
		return true
	case ".svg", ".png":
		return strings.HasPrefix(rel, "doc/assets/")
	default:
		return false
	}
}

func underRoot(root, abs string) bool {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}
