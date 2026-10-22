package testfixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNode materialises a scene node with the file roles a real node carries.
func writeNode(t *testing.T, root string) string {
	t.Helper()
	node := filepath.Join(root, "service-initialized", "node")
	files := map[string]string{
		"_meta.yaml":                    "views: [access-control]\nlayer: app\n",
		"_bundles.yaml":                 "bundles: []\n",
		"README.md":                     "# walkthrough\n",
		"construct.feature":             "Feature: construct\n",
		"_materials/drafts/schema.yaml": "type: object\n",
		"_materials/metric.gmv.json":    `{"name":"gmv"}`,
		"runbook.yaml":                  "steps: []\n",
		"subtitles.zh.vtt":              "WEBVTT\n",
	}
	for name, content := range files {
		path := filepath.Join(node, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return node
}

// TestSceneFingerprintIgnoresDocumentation is the measured boundary: a README edit
// under a node does not change index content, so it must not invalidate copies.
// Hashing the whole node would destroy every checkpoint on a docs edit.
func TestSceneFingerprintIgnoresDocumentation(t *testing.T) {
	node := writeNode(t, t.TempDir())
	before, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(node, "README.md"), []byte("# rewritten walkthrough\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("a README edit changed the fingerprint, so a documentation change would invalidate every checkpoint")
	}
}

// TestSceneFingerprintTracksDeclarations is the other half of the boundary: a
// declaration change alters index content, so reuse must be refused.
func TestSceneFingerprintTracksDeclarations(t *testing.T) {
	cases := map[string]string{
		"contract":    "_meta.yaml",
		"bundles":     "_bundles.yaml",
		"declaration": "_materials/drafts/schema.yaml",
		"material":    "_materials/metric.gmv.json",
		"probe":       "construct.feature",
		"other":       "runbook.yaml",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			node := writeNode(t, t.TempDir())
			before, err := SceneFingerprint(node)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(node, filepath.FromSlash(target))
			if err := os.WriteFile(path, []byte("changed: true\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			after, err := SceneFingerprint(node)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Fatalf("changing %s did not move the fingerprint, so a stale copy would look valid", target)
			}
		})
	}
}

// TestSceneFingerprintIgnoresMedia keeps narrated walkthrough assets out of the
// hash: they carry no index content and are large.
func TestSceneFingerprintIgnoresMedia(t *testing.T) {
	node := writeNode(t, t.TempDir())
	before, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(node, "subtitles.zh.vtt"), []byte("WEBVTT\n\n00:00.000 --> 00:05.000\nnarrated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("a subtitle edit changed the fingerprint")
	}
}

// TestSceneFingerprintIsStable keeps the value usable as a stored key.
func TestSceneFingerprintIsStable(t *testing.T) {
	node := writeNode(t, t.TempDir())
	first, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		again, err := SceneFingerprint(node)
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatal("fingerprint is not deterministic across calls")
		}
	}
	if strings.TrimSpace(first) == "" {
		t.Fatal("fingerprint is empty")
	}
}

// TestSceneFingerprintRejectsEmptyNode fails closed: a node with nothing to hash
// cannot produce a meaningful key.
func TestSceneFingerprintRejectsEmptyNode(t *testing.T) {
	empty := t.TempDir()
	if _, err := SceneFingerprint(empty); err == nil {
		t.Fatal("fingerprint accepted a node with no inputs")
	}
}

// TestSceneFingerprintSkipsGeneratedTrees keeps caches and results out: they are
// build output, not inputs, and they churn on every run.
func TestSceneFingerprintSkipsGeneratedTrees(t *testing.T) {
	node := writeNode(t, t.TempDir())
	before, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{"_results/report.json", "_build/construct.feature", "__pycache__/x.pyc"} {
		path := filepath.Join(node, filepath.FromSlash(generated))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("generated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	after, err := SceneFingerprint(node)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("generated output changed the fingerprint")
	}
}
