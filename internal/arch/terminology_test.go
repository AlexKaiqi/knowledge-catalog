package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicTerminologyHasNoRetiredWorkspaceAliases keeps the protocol,
// service design and exported runtime names aligned with docs/reviewed/terminology.md.
func TestPublicTerminologyHasNoRetiredWorkspaceAliases(t *testing.T) {
	root := moduleRoot(t)
	retired := []string{"WorkspaceView", "ResolvedView", "viewRef", "ViewLease", "Workspace Files API"}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".data", ".venv", ".kc", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == filepath.Join("docs", "reviewed", "terminology.md") || strings.HasSuffix(path, "_test.go") {
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

// TestKnowledgeSetSurfaceHasNoWorkspaceProtocolKeys fails if the protocol
// object still uses the retired Workspace short name on wire, packages, or
// scene inventory. User workdirs, git worktree, Store profile, and retired
// argv rejection tests are out of scope.
func TestKnowledgeSetSurfaceHasNoWorkspaceProtocolKeys(t *testing.T) {
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
			case ".git", ".venv", ".kc", "node_modules":
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
