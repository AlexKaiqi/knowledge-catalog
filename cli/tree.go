package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type treeFile struct {
	Path     string `json:"path"`
	ObjectID string `json:"objectId,omitempty"`
}

type treeRef struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

type treeRoot struct {
	Kind     string     `json:"kind"`
	ID       string     `json:"id,omitempty"`
	Dir      string     `json:"dir,omitempty"`
	Head     string     `json:"head,omitempty"`
	Archived bool       `json:"archived,omitempty"`
	Refs     []treeRef  `json:"refs,omitempty"`
	Files    []treeFile `json:"files,omitempty"`
}

func workspaceTree(home, as string) map[string]any {
	out := map[string]any{
		"roots": []treeRoot{},
	}
	file, err := ReadHome(home)
	if err != nil {
		return out
	}
	var catalogs []treeRoot
	for _, item := range file.Catalogs {
		if !catalogVisible(home, as, item.ID) {
			continue
		}
		root := gitTreeRoot(home, "catalog", item.ID, item.Dir)
		catalogs = append(catalogs, root)
	}
	var repos []treeRoot
	for _, item := range file.Repos {
		if !repoVisible(home, as, item.ID) {
			continue
		}
		root := gitTreeRoot(home, "repo", item.ID, item.Dir)
		for _, ref := range root.Refs {
			if ref.Name == "refs/kc/archived" {
				root.Archived = true
				break
			}
		}
		repos = append(repos, root)
	}
	out["roots"] = append(catalogs, repos...)
	return out
}

func catalogVisible(home, as, catalogID string) bool {
	if as == "" {
		return true
	}
	for _, cmd := range []string{"read-workspace", "read-catalog", "define-workspace", "audit"} {
		if PrincipalAllowed(home, as, cmd, "", catalogID) {
			return true
		}
	}
	return false
}

func repoVisible(home, as, repoID string) bool {
	if as == "" {
		return true
	}
	for _, cmd := range []string{"read", "list", "put", "log"} {
		if PrincipalAllowed(home, as, cmd, repoID, "") {
			return true
		}
	}
	return false
}

func gitTreeRoot(home, kind, id, rel string) treeRoot {
	dir := filepath.Join(home, rel)
	root := treeRoot{Kind: kind, ID: id, Dir: rel, Files: []treeFile{}, Refs: []treeRef{}}
	head, err := gitOutput(dir, "rev-parse", "HEAD")
	if err == nil {
		root.Head = head
		for _, path := range gitLines(dir, "ls-tree", "-r", "--name-only", "HEAD") {
			item := treeFile{Path: path}
			if body, err := gitOutput(dir, "show", "HEAD:"+path); err == nil {
				item.ObjectID = objectIDFromFrontmatter(body)
			}
			root.Files = append(root.Files, item)
		}
	}
	for _, line := range gitLines(dir, "for-each-ref", "--format=%(refname)%09%(objectname)") {
		name, commit, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		root.Refs = append(root.Refs, treeRef{Name: name, Commit: commit})
	}
	return root
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitLines(dir string, args ...string) []string {
	raw, err := gitOutput(dir, args...)
	if err != nil || raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func objectIDFromFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if line == "---" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "object_id" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func safeRelPath(home, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes home")
	}
	full := filepath.Join(home, clean)
	base := filepath.Clean(home)
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes home")
	}
	return full, nil
}
