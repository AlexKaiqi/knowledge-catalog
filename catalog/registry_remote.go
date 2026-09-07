package catalog

import (
	"net/url"
	"path/filepath"
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
	"kc/snapshot"
)

// OpenRemoteRegistry recovers an existing Catalog from its independent Git
// authority. rootDir is disposable cache; missing remote refs fail closed.
func OpenRemoteRegistry(rootDir, catalogID, remote, ref string) (*Registry, error) {
	g, err := remoteRegistry(rootDir, catalogID, remote, ref)
	if err != nil {
		return nil, err
	}
	head, err := g.dir.RemoteRef(g.remote, g.ref)
	if err != nil {
		return nil, err
	}
	if head == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "catalog authority ref does not exist")
	}
	g.head, err = g.dir.FetchRef(g.remote, g.ref)
	if err != nil {
		return nil, err
	}
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state.CatalogID != catalogID {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "catalog authority identity does not match requested catalog")
	}
	if err := stampCatalog(g.dir, catalogID); err != nil {
		return nil, err
	}
	if _, err := g.dir.Git("update-ref", g.ref, g.head); err != nil {
		return nil, err
	}
	if _, err := g.dir.Git("symbolic-ref", "HEAD", g.ref); err != nil {
		return nil, err
	}
	_ = g.dir.Materialize(g.head)
	return g, nil
}

// CreateRemoteRegistry creates only the Catalog ref and initial registry state
// in an already provisioned Git remote. It never creates a Knowledge Repository.
func CreateRemoteRegistry(rootDir, catalogID, remote, ref string) (*Registry, error) {
	g, err := remoteRegistry(rootDir, catalogID, remote, ref)
	if err != nil {
		return nil, err
	}
	head, err := g.dir.RemoteRef(g.remote, g.ref)
	if err != nil {
		return nil, err
	}
	if head != "" {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "catalog authority ref already exists; open it instead")
	}
	if err := stampCatalog(g.dir, catalogID); err != nil {
		return nil, err
	}
	if err := g.Save(CatalogState{CatalogID: catalogID}, "init "+catalogID, "", "", ""); err != nil {
		return nil, err
	}
	if _, err := g.dir.Git("symbolic-ref", "HEAD", g.ref); err != nil {
		return nil, err
	}
	return g, nil
}

func remoteRegistry(rootDir, catalogID, remote, ref string) (*Registry, error) {
	if strings.TrimSpace(catalogID) == "" || strings.TrimSpace(remote) == "" || strings.TrimSpace(rootDir) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog identity, Git remote, and cache directory are required")
	}
	if err := separateRegistryCache(rootDir, remote); err != nil {
		return nil, err
	}
	// Git runs in the cache directory; resolve filesystem remotes in the
	// caller's directory before passing them to Git.
	if !strings.Contains(remote, ":") {
		var err error
		remote, err = filepath.Abs(remote)
		if err != nil {
			return nil, err
		}
	}
	if ref == "" {
		ref = snapshot.DefaultRef
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog authority requires a branch ref")
	}
	dir, err := gitdir.OpenCache(rootDir)
	if err != nil {
		return nil, err
	}
	if !dir.OK("check-ref-format", ref) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "invalid catalog authority ref")
	}
	if existing, err := dir.Config(cfgCatalogID); err == nil && existing != "" && existing != catalogID {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "cache belongs to another catalog")
	}
	return &Registry{catalogID: catalogID, dir: dir, remote: remote, ref: ref}, nil
}

func separateRegistryCache(cache, remote string) error {
	location := remote
	if parsed, err := url.Parse(remote); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "file" {
			return nil
		}
		if parsed.Host != "" && parsed.Host != "localhost" {
			return kernel.Fail(kernel.ErrUsageInvalid, "file Git authority must be a local path")
		}
		location = parsed.Path
	} else if strings.Contains(remote, ":") {
		return nil
	} // SSH scp-style remote.
	cachePath, err := registryRealPath(cache)
	if err != nil {
		return err
	}
	remotePath, err := registryRealPath(location)
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{cachePath, remotePath}, {remotePath, cachePath}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return kernel.Fail(kernel.ErrUsageInvalid, "catalog authority and cache must be independent directories")
		}
	}
	return nil
}

func registryRealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs, nil
	}
	resolved, err := registryRealPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(abs)), nil
}

// CheckAuthority verifies that the instance still has the accepted Catalog
// branch. It neither refreshes state nor publishes anything.
func (g *Registry) CheckAuthority() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.remote == "" {
		return nil
	}
	actual, err := g.dir.RemoteRef(g.remote, g.ref)
	if err != nil {
		return err
	}
	if actual == "" {
		return kernel.Fail(kernel.ErrVersionUnresolved, "Catalog authority ref is unavailable")
	}
	if actual != g.head {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Catalog authority advanced; reopen the deployment")
	}
	return nil
}
