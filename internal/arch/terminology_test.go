package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicTerminologyHasNoRetiredWorkspaceAliases keeps the protocol,
// service design and exported runtime names aligned with docs/TERMINOLOGY.md.
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
		if rel == filepath.Join("docs", "TERMINOLOGY.md") || strings.HasSuffix(path, "_test.go") {
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
