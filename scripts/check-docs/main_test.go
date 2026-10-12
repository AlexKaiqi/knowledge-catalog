package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMarkdownLinksMaintainedDocuments(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		owner bool
	}{
		{name: "package README", path: "component/nested/README.md"},
		{name: "root task", path: "TASK.md"},
		{name: "migration record", path: "component/HTTP_REFACTOR.md"},
		{name: "graph owner", path: "docs/reviewed/topic.md", owner: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A checkout can itself have a hidden name; only child directories
			// are excluded from the maintained-document scan.
			root := filepath.Join(t.TempDir(), ".checkout")
			writeMarkdownFixture(t, root, tc.path, "[contract](contract.md)\n")
			docs := map[string]catalogEntry{}
			if tc.owner {
				docs["topic"] = catalogEntry{Path: tc.path}
			}
			err := validateMarkdownLinks(root, docs)
			if err == nil || !strings.Contains(err.Error(), tc.path+" -> contract.md") {
				t.Fatalf("broken maintained-document link was not reported: %v", err)
			}
			writeMarkdownFixture(t, root, filepath.Join(filepath.Dir(tc.path), "contract.md"), "contract\n")
			if err := validateMarkdownLinks(root, docs); err != nil {
				t.Fatalf("repaired maintained-document link failed: %v", err)
			}
		})
	}
}

func TestValidateMarkdownLinksIgnoresUnmaintainedTrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".checkout")
	writeMarkdownFixture(t, root, "README.md", "[contract](contract.md)\n")
	writeMarkdownFixture(t, root, "contract.md", "contract\n")
	for _, rel := range []string{
		".validation/README.md",
		"component/.cache/HTTP_REFACTOR.md",
		"node_modules/dependency/README.md",
		"component/node_modules/dependency/REFACTOR.md",
		"vendor/dependency/README.md",
		"dist/README.md",
		"dist-ssr/README.md",
		"build/README.md",
		"coverage/README.md",
		"test-results/README.md",
		"playwright-report/README.md",
		"component/TASK.md",
		"component/notes.md",
	} {
		writeMarkdownFixture(t, root, rel, "[stale reference](missing.md)\n")
	}
	if err := validateMarkdownLinks(root, nil); err != nil {
		t.Fatalf("unmaintained references contaminated the link check: %v", err)
	}
}

func writeMarkdownFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
