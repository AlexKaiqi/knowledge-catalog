package service

import (
	"kc/kernel"
	"path"
	"strings"
)

func RepositoryPath(subPath, relative string) (string, error) {
	joined := strings.Trim(path.Join(strings.Trim(subPath, "/"), strings.Trim(relative, "/")), "/")
	if relative != "" && (!ValidRelativePath(relative) || joined == "") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid mount-relative path %q", relative)
	}
	// Normalization must not move the result outside the selected subtree:
	// a relative path whose parent-directory segments clean away the subPath
	// prefix (e.g. "scratch/../..") escapes the published file list even
	// though it never begins with "../" (doc/design/dataset.md, C-21).
	if subPath != "" && joined != "" && joined != strings.Trim(subPath, "/") &&
		!strings.HasPrefix(joined, strings.Trim(subPath, "/")+"/") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "mount-relative path %q escapes the selected subtree", relative)
	}
	return joined, nil
}

func ValidRelativePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
