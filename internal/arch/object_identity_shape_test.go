package arch_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The object identity model (doc/design/object.md) names an object only by
// urn:kc:object:<namespace>/<name>, resolved inside the request scope, and a
// relation endpoint only by object ID. Repository-qualified URNs and
// objectRef endpoints are retired shapes; this guard keeps them from returning
// through code, fixtures, scene materials or documentation.
var retiredRepositoryURN = regexp.MustCompile(`urn:kc:object:[a-z0-9.-]+:/`)

func TestRetiredObjectIdentityShapesStayAbsent(t *testing.T) {
	root := moduleRoot(t)
	err := walkRepositorySource(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && identitySkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if strings.HasPrefix(filepath.ToSlash(rel), "tasks/done/") || !identityShapeExtensions[ext] {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		if match := retiredRepositoryURN.FindString(text); match != "" {
			t.Errorf("%s uses a retired Repository-qualified object URN %q; use urn:kc:object:<namespace>/<name>", rel, match)
		}
		if identityDataExtensions[ext] && strings.Contains(text, "endpoints") && strings.Contains(text, "objectRef") {
			t.Errorf("%s writes a relation endpoint as objectRef; endpoints are {role, object: <namespace>/<name>}", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// identitySkipDirs excludes only vendored, generated and local-state trees;
// scene materials under .data and design documents under doc are covered.
var identitySkipDirs = map[string]bool{
	".git": true, ".venv": true, "node_modules": true, ".kc": true, ".validation": true, ".session_tmps": true, "webui": true,
}

var identityShapeExtensions = map[string]bool{
	".go": true, ".md": true, ".yaml": true, ".yml": true, ".json": true, ".feature": true, ".py": true, ".ts": true, ".js": true, ".html": true,
}

var identityDataExtensions = map[string]bool{".yaml": true, ".yml": true, ".json": true, ".feature": true}
