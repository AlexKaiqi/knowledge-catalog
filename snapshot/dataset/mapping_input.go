package dataset

import (
	"fmt"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// ParseSourceMappings reads portable repository[=selector][@path[@subPath]] mappings.
// An omitted selector uses snapshot.DefaultRef. An omitted path leaves Path nil;
// a trailing @ declares the root mount, and a second @ selects the source subtree.
func ParseSourceMappings(items []string) ([]DatasetSource, error) {
	var sources []DatasetSource
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("--source requires an object source id")
		}
		repoSelector, rest, hasPath := strings.Cut(item, "@")
		repo, selector, ok := strings.Cut(repoSelector, "=")
		if !ok {
			repo = repoSelector
			selector = snapshot.DefaultRef
		} else if strings.TrimSpace(selector) == "" {
			selector = snapshot.DefaultRef
		}
		repo = strings.TrimSpace(repo)
		if repo == "" {
			return nil, fmt.Errorf("--source must be <repository>[=selector][@path[@subPath]], got %s", item)
		}
		src := DatasetSource{Repository: kernel.RepositoryID(repo), Selector: selector}
		if hasPath {
			path, subPath, _ := strings.Cut(rest, "@")
			src.Path = MountPath(path)
			src.SubPath = subPath
		}
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("at least one --source <repository>[=selector][@path] is required")
	}
	return sources, nil
}

// ParseFileMappings reads portable repository[=selector]@file@target mappings.
// Each mapping selects a source file and its delivered path; an empty input has no entries.
func ParseFileMappings(items []string) ([]DatasetSource, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]DatasetSource, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("--file-source requires <repository>[=<selector>]@<file>@<target>")
		}
		repoSelector, rest, ok := strings.Cut(item, "@")
		file, target, hasTarget := strings.Cut(rest, "@")
		if !ok || !hasTarget || strings.TrimSpace(file) == "" || strings.TrimSpace(target) == "" {
			return nil, fmt.Errorf("--file-source must be <repository>[=<selector>]@<file>@<target>, got %s", item)
		}
		repo, selector, _ := strings.Cut(repoSelector, "=")
		if strings.TrimSpace(selector) == "" {
			selector = snapshot.DefaultRef
		}
		repo = strings.TrimSpace(repo)
		if repo == "" {
			return nil, fmt.Errorf("--file-source must be <repository>[=<selector>]@<file>@<target>, got %s", item)
		}
		out = append(out, DatasetSource{
			Repository: kernel.RepositoryID(repo), Selector: selector, File: file, Target: target,
		})
	}
	return out, nil
}
