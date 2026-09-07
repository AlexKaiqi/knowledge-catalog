package catalog

import (
	"errors"
	"strings"
	"sync"

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
// Production Catalogs publish to their configured Git remote; RootDir is a disposable cache.
// History of those files is catalog.Log.
//
// The registry sits on internal/gitdir (plain git plumbing), not on a Snapshot
// adapter: layer ① stores its own authoritative Catalog data and never reads layer ② knowledge.
type Registry struct {
	catalogID string
	dir       *gitdir.Dir
	mu        sync.Mutex
	head      string
	remote    string
	ref       string
}

func NewRegistry(rootDir string, catalogID string) (*Registry, error) {
	dir, err := gitdir.Open(rootDir)
	if err != nil {
		return nil, err
	}
	if err := stampCatalog(dir, catalogID); err != nil {
		return nil, err
	}
	head, _ := dir.Rev(gitdir.BranchRef(gitdir.DefaultBranch))
	return &Registry{catalogID: catalogID, dir: dir, head: head, ref: gitdir.BranchRef(gitdir.DefaultBranch)}, nil
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.head == "" {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "catalog registry has no accepted commit")
	}
	return g.head, nil
}

// headYAML reads the flat top-level *.yaml files at HEAD. Nested paths are not
// registry files; the legacy layout used directories and is handled separately.
func (g *Registry) headYAML() (map[string][]byte, error) {
	head := g.head
	if head == "" {
		return map[string][]byte{}, nil
	}
	paths, err := g.dir.Paths(head)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".yaml") || strings.Contains(path, "/") {
			continue
		}
		body, err := g.dir.ShowRaw(head, path)
		if err != nil {
			return nil, err
		}
		out[path] = body
	}
	return out, nil
}

// Load reads the current flat Workspace registry YAML from HEAD.
func (g *Registry) Load() (CatalogState, error) {
	state, _, err := g.loadSnapshot()
	return state, err
}

func (g *Registry) loadSnapshot() (CatalogState, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state, err := g.load()
	return state, g.head, err
}

func (g *Registry) load() (CatalogState, error) {
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

// Save publishes state against this handle's last accepted authority commit.
// A stale handle must be reopened; Save never merges or overwrites newer state.
func (g *Registry) Save(state CatalogState, message, author, requestID, ruleID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.save(state, message, author, requestID, ruleID)
}

func (g *Registry) saveExpected(expected string, state CatalogState, message, author, requestID, ruleID string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if expected != g.head {
		return "", registryError(gitdir.ErrMoved{Ref: g.ref, Expected: expected, Actual: g.head})
	}
	err := g.save(state, message, author, requestID, ruleID)
	return g.head, err
}

func (g *Registry) save(state CatalogState, message, author, requestID, ruleID string) error {
	current, err := g.load()
	if err != nil {
		return err
	}
	if state.CatalogID != "" && state.CatalogID != g.catalogID {
		return kernel.Fail(kernel.ErrPreconditionFailed, "catalog state identity does not match registry")
	}
	next := NormalizeCatalogState(state)
	next.CatalogID = g.catalogID
	actual := ""
	if g.remote != "" {
		actual, err = g.dir.RemoteRef(g.remote, g.ref)
		if err != nil {
			return err
		}
	} else {
		actual, _ = g.dir.Rev(g.ref)
	}
	if actual != g.head {
		return registryError(gitdir.ErrMoved{Ref: g.ref, Expected: g.head, Actual: actual})
	}
	if kernel.CanonicalDigest(current) == kernel.CanonicalDigest(next) {
		return nil
	}
	if message == "" {
		message = "catalog: persist"
	}
	desired, err := yamlFiles(next, g.CatalogID())
	if err != nil {
		return err
	}
	candidate, err := g.dir.BuildFlatCommit(g.head, desired, gitdir.Signature{
		Author: author, Message: message, RequestID: requestID, RuleID: ruleID,
	})
	if err != nil {
		return err
	}
	if g.remote != "" {
		err = g.dir.PushCAS(g.remote, g.ref, candidate, g.head)
	} else {
		err = g.dir.CompareAndSwap(g.ref, candidate, g.head)
	}
	if err != nil {
		return registryError(err)
	}
	g.head = candidate
	if g.remote != "" {
		// Only a cache ref: its failure cannot revoke an accepted remote write.
		_, _ = g.dir.Git("update-ref", g.ref, candidate)
	}
	_ = g.dir.Materialize(candidate)
	return nil
}

func registryError(err error) error {
	var moved gitdir.ErrMoved
	if errors.As(err, &moved) {
		return kernel.Fail(kernel.ErrNonFastForward, "%s", moved.Error())
	}
	return err
}
