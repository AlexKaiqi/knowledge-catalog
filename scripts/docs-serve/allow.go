package main

import (
	"path/filepath"
	"strings"
)

func allowedRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return false
	}
	if rel == "README.md" {
		return true
	}
	if !strings.HasPrefix(rel, "docs/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".html":
		return true
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
