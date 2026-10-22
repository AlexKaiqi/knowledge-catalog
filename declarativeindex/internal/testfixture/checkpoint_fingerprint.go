package testfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/kernel"
)

// SceneFingerprint hashes the inputs that decide what a projection writes, and
// deliberately excludes the rest.
//
// The boundary is measured, not guessed. A scene node's index content derives
// from its contract (`_meta.yaml`, `_bundles.yaml`), the declarations and
// materials the projection submits, and the probe steps that drive it. A README
// or any other prose under the node does not reach the projection. Hashing the
// whole node would let one documentation edit invalidate every checkpoint, which
// removes the reason to keep them; hashing too little would let a changed
// declaration reuse stale documents that no count comparison can detect.
//
// The repository-wide digest used by validation is the wrong tool here for the
// same reason: it covers documentation, so every doc edit destroys all copies.
func SceneFingerprint(nodeDir string) (string, error) {
	inputs, err := sceneInputs(nodeDir)
	if err != nil {
		return "", err
	}
	if len(inputs) == 0 {
		return "", fmt.Errorf("scene node %s has no fingerprintable inputs", nodeDir)
	}
	digest := sha256.New()
	for _, path := range inputs {
		relative, err := filepath.Rel(nodeDir, path)
		if err != nil {
			return "", err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		digest.Write([]byte(filepath.ToSlash(relative)))
		digest.Write([]byte{0})
		digest.Write([]byte(classifySceneInput(path)))
		digest.Write([]byte{0})
		content := sha256.Sum256(payload)
		digest.Write(content[:])
	}
	return string(kernel.CanonicalDigest([]any{"scene-fingerprint-v1", hex.EncodeToString(digest.Sum(nil))})), nil
}

// sceneInputs collects the files whose content reaches the projection.
func sceneInputs(nodeDir string) ([]string, error) {
	var inputs []string
	err := filepath.Walk(nodeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "__pycache__", "_results", "_build":
				return filepath.SkipDir
			}
			return nil
		}
		if classifySceneInput(path) == "excluded" {
			return nil
		}
		inputs = append(inputs, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(inputs)
	return inputs, nil
}

// classifySceneInput names the role of a file inside a scene node. The returned
// class is hashed, so renaming a declaration to a different role changes the
// fingerprint even when its bytes are identical.
func classifySceneInput(path string) string {
	name := filepath.Base(path)
	if name == "README.md" || name == "readme.md" {
		return "excluded"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pyc", ".vtt", ".png", ".jpg", ".jpeg", ".gif", ".mp4", ".mov", ".svg":
		// Narrated walkthrough media never becomes index content.
		return "excluded"
	}
	switch {
	case name == "_meta.yaml" || name == "_bundles.yaml":
		return "contract"
	case strings.Contains(filepath.ToSlash(path), "/_materials/"):
		return "material"
	case strings.HasSuffix(name, ".feature"):
		return "probe"
	case strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml"):
		return "declaration"
	}
	// Anything else under a node is treated as content-bearing: over-covering an
	// unknown file rebuilds a copy, under-covering it silently reuses stale state.
	return "other"
}
