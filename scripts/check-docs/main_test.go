package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func documentationFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeMarkdownFixture(t, root, "go.mod", "module documentation-fixture\n")
	writeMarkdownFixture(t, root, "README.md", "# 入口\n\n[文档](doc/README.md)\n")
	writeMarkdownFixture(t, root, "doc/README.md", "# 文档\n\n[愿景](vision.md)、[对象](design/object.md)\n")
	writeMarkdownFixture(t, root, "doc/vision.md", "# 愿景\n\n见[对象](design/object.md)。\n")
	writeMarkdownFixture(t, root, "doc/design/object.md", "# 对象\n\n回到[愿景](../vision.md)。\n")
	return root
}

func TestMainAcceptsReachableDesignDocuments(t *testing.T) {
	root := documentationFixture(t)
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
	// Exercise the command, including shape, reachability and link checks.
	main()
}

func TestCheckDocumentationRejectsBrokenStructure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{"missing index", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "doc/README.md")); err != nil {
				t.Fatal(err)
			}
		}, "doc/README.md is missing"},
		{"unreachable document", func(t *testing.T, root string) {
			writeMarkdownFixture(t, root, "doc/design/orphan.md", "# 孤立\n")
		}, "doc/design/orphan.md"},
		{"frontmatter", func(t *testing.T, root string) {
			writeMarkdownFixture(t, root, "doc/vision.md", "---\ntitle: 愿景\n---\n# 愿景\n")
		}, "frontmatter"},
		{"missing title", func(t *testing.T, root string) {
			writeMarkdownFixture(t, root, "doc/vision.md", "愿景正文\n")
		}, "must start with a # title"},
		{"stray root Markdown", func(t *testing.T, root string) {
			writeMarkdownFixture(t, root, "DESIGN.md", "# 设计\n")
		}, "root Markdown DESIGN.md"},
		{"broken design link", func(t *testing.T, root string) {
			writeMarkdownFixture(t, root, "doc/vision.md", "# 愿景\n\n[旧文档](../docs/product.md)、[对象](design/object.md)\n")
		}, "doc/vision.md -> ../docs/product.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := documentationFixture(t)
			if _, err := checkDocumentation(root); err != nil {
				t.Fatalf("baseline fixture failed: %v", err)
			}
			tc.setup(t, root)
			_, err := checkDocumentation(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestReachabilityFollowsLinksBetweenDesignDocuments(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "doc/design/plugins.md", "# 插件\n")
	writeMarkdownFixture(t, root, "doc/design/object.md", "# 对象\n\n[插件](plugins.md)\n")
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("document linked through another design document is reachable: %v", err)
	}
}

func TestValidateMarkdownLinksMaintainedDocuments(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "package README", path: "component/nested/README.md"},
		{name: "task", path: "tasks/todo/PLUGIN-INDEX-01.md"},
		{name: "agent rules", path: "AGENTS.md"},
		{name: "maintained note", path: "cli/SURFACE.md"},
		{name: "design document", path: "doc/design/topic.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A checkout can itself have a hidden name; only child directories
			// are excluded from the maintained-document scan.
			root := filepath.Join(t.TempDir(), ".checkout")
			writeMarkdownFixture(t, root, tc.path, "[contract](contract.md)\n")
			var docs []string
			if strings.HasPrefix(tc.path, "doc/") || strings.HasPrefix(tc.path, "tasks/") {
				docs = []string{tc.path}
			}
			check := func() error {
				files, err := markdownLinkFiles(root, docs)
				if err != nil {
					t.Fatal(err)
				}
				return validateMarkdownLinks(root, files)
			}
			if err := check(); err == nil || !strings.Contains(err.Error(), tc.path+" -> contract.md") {
				t.Fatalf("broken maintained-document link was not reported: %v", err)
			}
			writeMarkdownFixture(t, root, filepath.Join(filepath.Dir(tc.path), "contract.md"), "contract\n")
			if err := check(); err != nil {
				t.Fatalf("repaired maintained-document link failed: %v", err)
			}
		})
	}
}

func TestValidateMarkdownLinksRejectsLocalValidationEvidence(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), ".checkout")
			writeMarkdownFixture(t, root, "README.md", "[evidence](.validation/run/report.json)\n")
			if present {
				writeMarkdownFixture(t, root, ".validation/run/report.json", "{}\n")
			}
			if err := validateMarkdownLinks(root, []string{"README.md"}); err == nil || !strings.Contains(err.Error(), "local validation evidence is not checked in") {
				t.Fatalf("local-only evidence link was not rejected: %v", err)
			}
			writeMarkdownFixture(t, root, "README.md", "Evidence: `.validation/run/report.json`.\n")
			if err := validateMarkdownLinks(root, []string{"README.md"}); err != nil {
				t.Fatalf("literal evidence path rejected: %v", err)
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
		"component/.cache/README.md",
		"node_modules/dependency/README.md",
		"component/node_modules/dependency/README.md",
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
	files, err := markdownLinkFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMarkdownLinks(root, files); err != nil {
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
