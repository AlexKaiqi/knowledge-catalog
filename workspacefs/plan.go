// Package workspacefs projects one resolved Catalog Workspace into host
// filesystem mountpoints. It is an application adapter above Catalog and
// Snapshot: the package owns no repository, knowledge, or write semantics.
package workspacefs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// File is one immutable file in a resolved Workspace mount. Path is relative
// to the mount root and Read must keep reading the same resolved commit for the
// lifetime of the mount session.
type File struct {
	Path string
	Read func() ([]byte, error)
}

// Mount maps one Workspace member subtree to one path below the user's
// existing project root.
type Mount struct {
	Path       string
	Repository string
	Commit     string
	Files      []File
}

// Plan is the immutable input to a host mount session.
type Plan struct {
	WorkspaceID string
	PinID       string
	Root        string
	Mounts      []Mount
}

// Target is a validated mountpoint and its immutable file tree.
type Target struct {
	Mountpoint  string
	Mount       Mount
	root        *tree
	projectRoot string
}

type tree struct {
	dirs  map[string]*tree
	files map[string]File
}

func newTree() *tree {
	return &tree{dirs: map[string]*tree{}, files: map[string]File{}}
}

// Validate checks path ownership without touching the host filesystem.
func (p Plan) Validate() ([]Target, error) {
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if strings.TrimSpace(p.Root) == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	if len(p.Mounts) == 0 {
		return nil, fmt.Errorf("workspace %s has no visible mounts", p.WorkspaceID)
	}
	root, err := filepath.Abs(p.Root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %s is not a directory", root)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root %s: %w", root, err)
	}
	seen := map[string]string{}
	targets := make([]Target, 0, len(p.Mounts))
	for _, mount := range p.Mounts {
		if !validRawRelative(mount.Path) {
			return nil, fmt.Errorf("invalid mount path %q", mount.Path)
		}
		mount.Path = cleanRelative(mount.Path)
		if mount.Path == "" {
			return nil, fmt.Errorf("repository %s declares a root mount; attaching to an existing project requires a non-root Path", mount.Repository)
		}
		if !validRelative(mount.Path) {
			return nil, fmt.Errorf("invalid mount path %q", mount.Path)
		}
		mountpoint := filepath.Join(root, filepath.FromSlash(mount.Path))
		relTarget, err := filepath.Rel(root, mountpoint)
		if err != nil || relTarget == "." || relTarget == ".." || strings.HasPrefix(relTarget, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("mount path %q escapes workspace root", mount.Path)
		}
		if _, err := missingDirectories(root, mountpoint); err != nil {
			return nil, err
		}
		for priorPath, priorRepo := range seen {
			if mount.Path == priorPath || strings.HasPrefix(mount.Path, priorPath+"/") || strings.HasPrefix(priorPath, mount.Path+"/") {
				return nil, fmt.Errorf("mount path %q for %s overlaps %q for %s", mount.Path, mount.Repository, priorPath, priorRepo)
			}
		}
		seen[mount.Path] = mount.Repository
		rootNode := newTree()
		for _, file := range mount.Files {
			if file.Read == nil {
				return nil, fmt.Errorf("mount %s file %q has no reader", mount.Path, file.Path)
			}
			if err := rootNode.add(file); err != nil {
				return nil, fmt.Errorf("mount %s: %w", mount.Path, err)
			}
		}
		sort.Slice(mount.Files, func(i, j int) bool { return mount.Files[i].Path < mount.Files[j].Path })
		targets = append(targets, Target{Mountpoint: mountpoint, Mount: mount, root: rootNode, projectRoot: root})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Mount.Path < targets[j].Mount.Path })
	return targets, nil
}

func (t *tree) add(file File) error {
	if !validRawRelative(file.Path) {
		return fmt.Errorf("invalid file path %q", file.Path)
	}
	clean := cleanRelative(file.Path)
	if clean == "" || !validRelative(clean) {
		return fmt.Errorf("invalid file path %q", file.Path)
	}
	parts := strings.Split(clean, "/")
	at := t
	for _, part := range parts[:len(parts)-1] {
		if _, conflict := at.files[part]; conflict {
			return fmt.Errorf("path %q is both a file and a directory", clean)
		}
		next := at.dirs[part]
		if next == nil {
			next = newTree()
			at.dirs[part] = next
		}
		at = next
	}
	name := parts[len(parts)-1]
	if _, conflict := at.dirs[name]; conflict {
		return fmt.Errorf("path %q is both a file and a directory", clean)
	}
	if _, duplicate := at.files[name]; duplicate {
		return fmt.Errorf("duplicate file path %q", clean)
	}
	file.Path = clean
	at.files[name] = file
	return nil
}

func cleanRelative(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	if trimmed == "" || trimmed == "." {
		return ""
	}
	return path.Clean(trimmed)
}

func validRelative(value string) bool {
	return value != "" && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsRune(value, '\x00') && !strings.Contains(value, "\\")
}

func validRawRelative(value string) bool {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "/") || strings.ContainsRune(trimmed, '\x00') || strings.Contains(trimmed, "\\") {
		return false
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	return true
}

// missingDirectories also validates every existing path component. In
// particular, a recipe cannot escape the validated project root through a
// symlink that happens to sit below it.
func missingDirectories(root, target string) ([]string, error) {
	var reversed []string
	for at := target; at != root; at = filepath.Dir(at) {
		info, err := os.Lstat(at)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("mount path traverses symlink %s", at)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("mount path parent %s is not a directory", at)
			}
		case os.IsNotExist(err):
			reversed = append(reversed, at)
		default:
			return nil, err
		}
	}
	created := make([]string, len(reversed))
	for i := range reversed {
		created[i] = reversed[len(reversed)-1-i]
	}
	return created, nil
}
