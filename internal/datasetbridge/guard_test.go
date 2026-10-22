package datasetbridge_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A second field mapping would silently drift again: the two copies that existed
// before this package both dropped Dependencies. This guard fails on a new
// literal DatasetPin construction from a resolution, so a new consumer must call
// Pin instead of copying fields.
func TestOnlyOneDatasetPinFieldMapping(t *testing.T) {
	root := moduleRoot(t)
	// Assigning the shared shapes field by field is the shape of a copy: either
	// a composite literal keyed by SetID/Revision/Repositories, or a loop that
	// appends DatasetItem values from a resolution.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`reader\.DatasetPin\{[^}]*SetID:\s*\w+\.SetID`),
		regexp.MustCompile(`DatasetItem\{Target:\s*\w+\.Target,\s*Repository:`),
	}
	allowed := map[string]bool{
		// The one implementation owns the mapping.
		filepath.Join("internal", "datasetbridge", "pin.go"): true,
		// This test states the forbidden shapes in its own patterns.
		filepath.Join("internal", "datasetbridge", "pin_test.go"): true,
	}
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if allowed[relative] {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Cheap prefilter: a copy must mention the pin or item type before any
		// regex is worth running.
		if !bytes.Contains(raw, []byte("DatasetPin{")) && !bytes.Contains(raw, []byte("DatasetItem{")) {
			return nil
		}
		for _, pattern := range patterns {
			if pattern.Match(raw) {
				offenders = append(offenders, relative)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("DatasetPin reached a second field mapping; call datasetbridge.Pin instead: %v", offenders)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".validation", "node_modules", "vendor", "testdata":
		return true
	}
	return false
}
