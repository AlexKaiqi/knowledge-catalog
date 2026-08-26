package workspacefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanAllowsMultipleIndependentMounts(t *testing.T) {
	plan := Plan{
		WorkspaceID: "agent",
		Root:        t.TempDir(),
		Mounts: []Mount{
			{Path: "knowledge/team", Repository: "team", Files: []File{{Path: "README.md", Read: bytesReader("team")}}},
			{Path: "vendor/policy", Repository: "policy", Files: []File{{Path: "docs/rules.md", Read: bytesReader("rules")}}},
		},
	}
	targets, err := plan.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets", len(targets))
	}
	realRoot, err := filepath.EvalSymlinks(plan.Root)
	if err != nil {
		t.Fatal(err)
	}
	if got := targets[0].Mountpoint; got != filepath.Join(realRoot, "knowledge", "team") {
		t.Fatalf("mountpoint = %s", got)
	}
}

func TestPlanRejectsRootAndOverlappingMounts(t *testing.T) {
	for name, mounts := range map[string][]Mount{
		"root": {{Path: "", Repository: "root"}},
		"nested": {
			{Path: "knowledge", Repository: "one"},
			{Path: "knowledge/team", Repository: "two"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (Plan{WorkspaceID: "agent", Root: t.TempDir(), Mounts: mounts}).Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPlanRejectsFileDirectoryCollision(t *testing.T) {
	_, err := (Plan{
		WorkspaceID: "agent",
		Root:        t.TempDir(),
		Mounts: []Mount{{Path: "knowledge", Repository: "one", Files: []File{
			{Path: "a", Read: bytesReader("a")},
			{Path: "a/b", Read: bytesReader("b")},
		}}},
	}).Validate()
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestPlanRejectsAbsoluteTraversalAndSymlinkMounts(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	for name, mountPath := range map[string]string{
		"absolute":  "/outside",
		"traversal": "knowledge/../outside",
		"symlink":   "linked/knowledge",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (Plan{WorkspaceID: "agent", Root: root, Mounts: []Mount{{Path: mountPath, Repository: "one"}}}).Validate()
			if err == nil {
				t.Fatal("expected unsafe path to fail")
			}
		})
	}
}

func bytesReader(value string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(value), nil }
}
