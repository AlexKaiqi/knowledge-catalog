package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainAcceptsDesignDocumentsWithoutFixedSections(t *testing.T) {
	root := t.TempDir()
	writeMarkdownFixture(t, root, "go.mod", "module documentation-fixture\n")
	for _, class := range []string{"entrypoint", "foundation", "decision", "runtime", "evolution"} {
		path := "docs/reviewed/" + class + ".md"
		if class == "entrypoint" {
			path = "README.md"
		}
		writeMarkdownFixture(t, root, path, "# 重构目标\n\n沿已有契约接入插件，读取时返回实际数据。\n")
		body, err := json.Marshal(catalogEntry{
			ID: class, Path: path, Title: "重构目标", Class: class,
			Lifecycle: "active", OwnerTopics: []string{class + "-topic"},
		})
		if err != nil {
			t.Fatal(err)
		}
		unit := fmt.Sprintf("---\nobject_id: documentation/%s\naspect_name: catalog-entry\nkind: Aspect\n---\n%s\n", class, body)
		writeMarkdownFixture(t, root, "docs/graph/documents/"+class+".okf", unit)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	// Exercise the command, including graph and link checks. Reintroducing a
	// mandatory section here must fail even when lower-level checks still pass.
	main()
}

func TestValidateCatalogKeepsOwnershipAndCycleChecks(t *testing.T) {
	root := t.TempDir()
	writeMarkdownFixture(t, root, "README.md", "入口\n")
	writeMarkdownFixture(t, root, "docs/topic.md", "目标：透明读取。\n")
	docs := map[string]catalogEntry{
		"root":  {Path: "README.md", OwnerTopics: []string{"entry"}},
		"topic": {Path: "docs/topic.md", OwnerTopics: []string{"entry"}},
	}
	if err := validateCatalog(root, docs, nil); err == nil || !strings.Contains(err.Error(), `ownerTopic "entry" duplicated`) {
		t.Fatalf("duplicate topic ownership was not rejected: %v", err)
	}
	docs["topic"] = catalogEntry{Path: "docs/topic.md", OwnerTopics: []string{"topic"}}
	if err := validateCatalog(root, docs, nil); err != nil {
		t.Fatalf("distinct ownership failed: %v", err)
	}
	cycle := []edge{
		{Type: "depends_on", From: "root", To: "topic"},
		{Type: "depends_on", From: "topic", To: "root"},
	}
	if err := validateCatalog(root, docs, cycle); err == nil || !strings.Contains(err.Error(), "depends_on graph has a cycle") {
		t.Fatalf("dependency cycle was not rejected: %v", err)
	}
}

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
