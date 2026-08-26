package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
)

// cfgCatalogID labels a registry directory. Deliberately not kc.repositoryId:
// a Catalog registry is not a Knowledge Repository, and Repository discovery must not
// mistake one for the other. The authoritative id is catalog.yaml at HEAD; this
// stamp only answers "whose registry is this" before the first commit.
const cfgCatalogID = "kc.catalogId"

// Registry persists WorkspaceDefinition and registered repositories as flat YAML
// files in the registry git root (catalog.yaml, workspace-*.yaml, repository-*.yaml).
//
// It is not a Knowledge Repository. Do not `repo-add` a Catalog id into a Workspace.
// On disk each Catalog is <home>/catalogs/<encoded-id> (layout.catalogs).
// History of those files is catalog.Log.
//
// The registry sits on internal/gitdir (plain git plumbing), not on a Snapshot
// adapter: layer ① stores its own config files and never reads layer ② knowledge.
type Registry struct {
	catalogID string
	dir       *gitdir.Dir
}

func NewRegistry(rootDir string, catalogID string) (*Registry, error) {
	dir, err := gitdir.Open(rootDir)
	if err != nil {
		return nil, err
	}
	if err := stampCatalog(dir, catalogID); err != nil {
		return nil, err
	}
	return &Registry{catalogID: catalogID, dir: dir}, nil
}

func stampCatalog(dir *gitdir.Dir, catalogID string) error {
	existing, err := dir.Config(cfgCatalogID)
	if err == nil && existing != "" && existing != catalogID {
		return kernel.Fail(kernel.ErrPreconditionFailed,
			"directory %s is stamped as catalog %s, not %s", dir.Root(), existing, catalogID)
	}
	if existing == catalogID {
		return nil
	}
	return dir.SetConfig(cfgCatalogID, catalogID)
}

func (g *Registry) CatalogID() string { return g.catalogID }

// RootDir is the registry git working directory.
func (g *Registry) RootDir() string { return g.dir.Root() }

// Head is the registry commit the current combination space was read from.
func (g *Registry) Head() (string, error) {
	commit, ok := g.dir.Rev(gitdir.BranchRef(gitdir.DefaultBranch))
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "registry %s has no %s", g.dir.Root(), gitdir.DefaultBranch)
	}
	return commit, nil
}

// headYAML reads the flat top-level *.yaml files at HEAD. Nested paths are not
// registry files; the legacy layout used directories and is handled separately.
func (g *Registry) headYAML() (map[string][]byte, error) {
	head, ok := g.dir.Rev(gitdir.BranchRef(gitdir.DefaultBranch))
	if !ok {
		return map[string][]byte{}, nil
	}
	paths, err := g.dir.Paths(head)
	if err != nil {
		return map[string][]byte{}, nil
	}
	out := map[string][]byte{}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".yaml") || strings.Contains(path, "/") {
			continue
		}
		body, err := g.dir.Show(head, path)
		if err != nil {
			return nil, err
		}
		out[path] = []byte(body + "\n")
	}
	return out, nil
}

// Load reads the current flat Workspace registry YAML from HEAD.
func (g *Registry) Load() (CatalogState, error) {
	files, err := g.headYAML()
	if err != nil {
		return CatalogState{}, err
	}
	if len(files) > 0 {
		return g.stateFromYAML(files)
	}
	return EmptyCatalogState, nil
}

func (g *Registry) stateFromYAML(files map[string][]byte) (CatalogState, error) {
	workspaces := []WorkspaceDefinition{}
	ids := []string{}
	archived := false
	catalogID := ""
	for path, body := range files {
		switch {
		case path == CatalogFile():
			var meta catalogMeta
			if err := decodeYAML(body, &meta); err != nil {
				return CatalogState{}, err
			}
			archived = meta.Archived
			catalogID = meta.ID
		case strings.HasPrefix(path, workspaceFilePrefix):
			def, err := asWorkspaceDefinitionYAML(body)
			if err != nil {
				return CatalogState{}, err
			}
			workspaces = append(workspaces, def)
		case strings.HasPrefix(path, repositoryFilePrefix):
			var row map[string]string
			if err := decodeYAML(body, &row); err != nil {
				return CatalogState{}, err
			}
			if id := row["repository"]; id != "" {
				ids = append(ids, id)
			} else if id := row["repositoryId"]; id != "" {
				ids = append(ids, id)
			}
		}
	}
	return NormalizeCatalogState(CatalogState{
		Workspaces: workspaces, Repositories: ids, Archived: archived,
		CatalogID: catalogID,
	}), nil
}

// Save diffs CatalogState against HEAD and commits YAML files. Same digest is a no-op once YAML is the layout.
func (g *Registry) Save(state CatalogState, message, author, requestID, ruleID string) error {
	current, err := g.Load()
	if err != nil {
		return err
	}
	next := NormalizeCatalogState(state)
	files, err := g.headYAML()
	if err != nil {
		return err
	}
	if kernel.CanonicalDigest(current) == kernel.CanonicalDigest(next) && len(files) > 0 {
		return nil
	}
	if message == "" {
		message = "catalog: persist"
	}
	desired, err := yamlFiles(next, g.CatalogID())
	if err != nil {
		return err
	}
	head, err := g.Head()
	if err != nil {
		return err
	}
	if err := g.dir.Checkout(gitdir.DefaultBranch); err != nil {
		return err
	}
	if err := g.writeFlatLayout(desired); err != nil {
		return err
	}
	_, err = g.dir.CommitWorktree(head, gitdir.Signature{
		Author: author, Message: message, RequestID: requestID, RuleID: ruleID,
	})
	var moved gitdir.ErrMoved
	if errors.As(err, &moved) {
		return kernel.Fail(kernel.ErrNonFastForward, "%s", moved.Error())
	}
	return err
}

// writeFlatLayout makes the top-level Workspace registry YAML exactly match the
// desired state. Unrecognized directories are not interpreted or deleted.
func (g *Registry) writeFlatLayout(desired map[string][]byte) error {
	root := g.dir.Root()
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if _, ok := desired[e.Name()]; !ok {
			if err := os.Remove(filepath.Join(root, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, body := range desired {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
