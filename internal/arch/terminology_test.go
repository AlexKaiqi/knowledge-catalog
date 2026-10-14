package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicTerminologyHasNoRetiredWorkspaceAliases keeps the protocol,
// service design and exported runtime names aligned with doc/concepts.md.
func TestPublicTerminologyHasNoRetiredWorkspaceAliases(t *testing.T) {
	root := moduleRoot(t)
	retired := []string{"WorkspaceView", "ResolvedView", "viewRef", "ViewLease", "Workspace Files API"}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".validation", ".session_tmps", ".git", ".data", ".venv", ".kc", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == filepath.Join("doc", "concepts.md") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".md" && ext != ".ts" && ext != ".tsx" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, term := range retired {
			if strings.Contains(string(body), term) {
				t.Errorf("retired term %q remains in %s", term, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestObjectIsTheOnlyNameForCatalogContent keeps every tracked source, script,
// fixture and document on the object vocabulary of doc/concepts.md. The
// product name and the English verb "acknowledge" are the only exemptions;
// concepts.md keeps the retired word in its "not used" table and TASK.md is a
// historical ledger. The retired words are assembled at runtime so this file
// does not exempt itself.
func TestObjectIsTheOnlyNameForCatalogContent(t *testing.T) {
	root := moduleRoot(t)
	retired := "know" + "ledge"
	retiredZH := "知" + "识"
	exempt := []string{"ac" + retired, retired + "-catalog", retired + "_catalog", retired + " catalog", retiredZH + "底座"}
	exemptFiles := map[string]bool{"doc/concepts.md": true, "TASK.md": true}
	textual := map[string]bool{".go": true, ".md": true, ".ts": true, ".tsx": true, ".feature": true, ".yaml": true, ".yml": true, ".json": true, ".py": true, ".sh": true, ".sql": true, ".mdc": true}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		slash := filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".validation", ".session_tmps", ".git", ".venv", ".kc", "node_modules", "dist", "_results":
				return filepath.SkipDir
			}
		}
		if path != root && strings.Contains(strings.ToLower(d.Name()), retired) {
			t.Errorf("retired name in path %s", slash)
		}
		if d.IsDir() || exemptFiles[slash] || !textual[filepath.Ext(path)] {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := strings.ToLower(string(body))
		for _, word := range exempt {
			text = strings.ReplaceAll(text, strings.ToLower(word), "")
		}
		for _, word := range []string{retired, retiredZH} {
			if i := strings.Index(text, word); i >= 0 {
				t.Errorf("retired word %q remains in %s near %q", word, slash, text[max(0, i-30):min(len(text), i+40)])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDatasetSurfaceHasNoWorkspaceProtocolKeys fails if the protocol
// object still uses the retired Workspace short name on wire, packages, or
// scene inventory. User workdirs, git worktree, Store profile, and retired
// argv rejection tests are out of scope.
func TestDatasetSurfaceHasNoWorkspaceProtocolKeys(t *testing.T) {
	root := moduleRoot(t)
	forbidden := []string{
		`json:"workspace"`,
		`json:"workspace,omitempty"`,
		"package workspacefs",
		`"kc/workspacefs"`,
		"workspaces[].setId",
		"| workspaces |",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		slash := filepath.ToSlash(rel)
		if slash == filepath.ToSlash(filepath.Join("internal", "arch", "terminology_test.go")) {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".validation", ".session_tmps", ".git", ".venv", ".kc", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".feature" && ext != ".md" && ext != ".yaml" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, term := range forbidden {
			if strings.Contains(text, term) {
				t.Errorf("retired protocol key %q remains in %s", term, slash)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
