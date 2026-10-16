package arch_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Guard the same source boundary as validation: tracked files and new,
// non-ignored files. Local trial dumps are not protocol source, while tracked
// fixtures remain covered even when a later ignore rule matches their path.
func walkRepositorySource(root string, visit fs.WalkDirFunc) error {
	command := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	raw, err := command.Output()
	if err != nil {
		return err
	}
	files := map[string]bool{}
	directories := map[string]bool{".": true}
	for _, name := range strings.Split(string(raw), "\x00") {
		if name == "" {
			continue
		}
		name = filepath.FromSlash(name)
		files[name] = true
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			directories[parent] = true
		}
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && !directories[relative] {
			return filepath.SkipDir
		}
		if !entry.IsDir() && !files[relative] {
			return nil
		}
		return visit(path, entry, nil)
	})
}

func TestArchitectureSourceInventoryKeepsFixturesAndNewFiles(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	git("init", "--quiet")
	write := func(name, value string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", ".data/*\n!.data/scenes/\n!.data/scale/\n.validation/\n")
	write(".data/scenes/new.feature", "protocol fixture")
	write(".data/scale/new.yaml", "scale fixture")
	write(".data/tracked.yaml", "tracked despite ignore rule")
	git("add", "--force", ".data/tracked.yaml")
	write("new.go", "package fixture")
	write(".data/realcase/output.yaml", "ignored local trial")
	write(".validation/output.json", "ignored validation evidence")
	seen := map[string]bool{}
	if err := walkRepositorySource(root, func(path string, entry fs.DirEntry, _ error) error {
		if !entry.IsDir() {
			relative, _ := filepath.Rel(root, path)
			seen[filepath.ToSlash(relative)] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".gitignore", ".data/scenes/new.feature", ".data/scale/new.yaml", ".data/tracked.yaml", "new.go"} {
		if !seen[path] {
			t.Errorf("source guard omitted %s", path)
		}
	}
	for _, path := range []string{".data/realcase/output.yaml", ".validation/output.json"} {
		if seen[path] {
			t.Errorf("source guard included local output %s", path)
		}
	}
}
