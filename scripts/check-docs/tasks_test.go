package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func taskBoardFixture(t *testing.T) string {
	t.Helper()
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "tasks/README.md", "# 任务\n\n[设计](../doc/README.md)\n")
	writeMarkdownFixture(t, root, "tasks/todo/PLUGIN-INTERPRETER-01.md", "# PLUGIN-INTERPRETER-01 · 插件按形状认领\n\n[对象](../../doc/design/object.md)\n")
	writeMarkdownFixture(t, root, "tasks/doing/.gitkeep", "")
	writeMarkdownFixture(t, root, "tasks/done/.gitkeep", "")
	return root
}

func TestCheckDocumentationAcceptsTaskBoard(t *testing.T) {
	root := taskBoardFixture(t)
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("valid task board rejected: %v", err)
	}
	// A checkout without a board is still valid.
	if _, err := checkDocumentation(documentationFixture(t)); err != nil {
		t.Fatalf("checkout without tasks/ rejected: %v", err)
	}
}

func TestCheckDocumentationAcceptsExplorations(t *testing.T) {
	root := taskBoardFixture(t)
	writeMarkdownFixture(t, root, "tasks/explore/PERM-GATE-01.md", "# PERM-GATE-01 · 权限门禁在各入口是否一致\n\n[设计](../../doc/README.md)\n")
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("exploration rejected: %v", err)
	}
	// An empty explore/ directory is allowed: explorations come and go.
	removeFixture(t, root, "tasks/explore")
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("board without explore/ rejected: %v", err)
	}
}

func TestCheckDocumentationRejectsBrokenExplorations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		body  string
		cause string
	}{
		{name: "bad id", path: "tasks/explore/perm-gate.md", body: "# perm-gate\n", cause: "tasks/explore/perm-gate.md is not an exploration file"},
		{name: "not markdown", path: "tasks/explore/PERM-GATE-01.txt", body: "x\n", cause: "is not an exploration file"},
		{name: "nested", path: "tasks/explore/group/PERM-GATE-01.md", body: "# PERM-GATE-01\n", cause: "tasks/explore/group is not an exploration file"},
		{name: "title mismatch", path: "tasks/explore/PERM-GATE-01.md", body: "# PERM-GATE-02 · 别的探索\n", cause: "must start with the title \"# PERM-GATE-01"},
		{name: "shares task id", path: "tasks/explore/PLUGIN-INTERPRETER-01.md", body: "# PLUGIN-INTERPRETER-01 · 同名\n", cause: "PLUGIN-INTERPRETER-01 is also a task in todo"},
		{name: "broken link", path: "tasks/explore/PERM-GATE-01.md", body: "# PERM-GATE-01\n\n[缺](../../doc/missing.md)\n", cause: "tasks/explore/PERM-GATE-01.md -> ../../doc/missing.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := taskBoardFixture(t)
			writeMarkdownFixture(t, root, tc.path, tc.body)
			_, err := checkDocumentation(root)
			if err == nil || !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("expected %q, got %v", tc.cause, err)
			}
		})
	}
}

func TestCheckDocumentationRejectsBrokenTaskBoard(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		body  string
		cause string
	}{
		{name: "unknown state", path: "tasks/blocked/PLUGIN-INDEX-01.md", body: "# PLUGIN-INDEX-01\n", cause: "tasks/blocked is not allowed"},
		{name: "loose task", path: "tasks/PLUGIN-INDEX-01.md", body: "# PLUGIN-INDEX-01\n", cause: "tasks/PLUGIN-INDEX-01.md is not allowed"},
		{name: "bad id", path: "tasks/todo/plugin-index.md", body: "# plugin-index\n", cause: "tasks/todo/plugin-index.md is not a task file"},
		{name: "not markdown", path: "tasks/todo/PLUGIN-INDEX-01.txt", body: "x\n", cause: "is not a task file"},
		{name: "nested", path: "tasks/todo/group/PLUGIN-INDEX-01.md", body: "# PLUGIN-INDEX-01\n", cause: "tasks/todo/group is not a task file"},
		{name: "title mismatch", path: "tasks/todo/PLUGIN-INDEX-01.md", body: "# PLUGIN-INDEX-02 · 别的任务\n", cause: "must start with the title \"# PLUGIN-INDEX-01"},
		{name: "two states", path: "tasks/done/PLUGIN-INTERPRETER-01.md", body: "# PLUGIN-INTERPRETER-01\n", cause: "task PLUGIN-INTERPRETER-01 is also in todo"},
		{name: "broken link", path: "tasks/todo/PLUGIN-INDEX-01.md", body: "# PLUGIN-INDEX-01\n\n[缺](../../doc/missing.md)\n", cause: "tasks/todo/PLUGIN-INDEX-01.md -> ../../doc/missing.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := taskBoardFixture(t)
			writeMarkdownFixture(t, root, tc.path, tc.body)
			_, err := checkDocumentation(root)
			if err == nil || !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("expected %q, got %v", tc.cause, err)
			}
		})
	}
}

func TestCheckDocumentationRequiresTaskBoardParts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove string
		cause  string
	}{
		{name: "index", remove: "tasks/README.md", cause: "tasks/README.md is missing"},
		{name: "state directory", remove: "tasks/doing", cause: "tasks/doing is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := taskBoardFixture(t)
			removeFixture(t, root, tc.remove)
			_, err := checkDocumentation(root)
			if err == nil || !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("expected %q, got %v", tc.cause, err)
			}
		})
	}
}

func removeFixture(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}
