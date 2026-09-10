package catalog

import (
	"kc/kernel"
	"kc/snapshot"
)

type catalogMeta struct {
	ID       string `json:"id,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

func (c *Catalog) Repositories() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.repositoriesList()
}

func (c *Catalog) repositoriesList() []string {
	out := make([]string, 0, len(c.repositories))
	for id := range c.repositories {
		out = append(out, id)
	}
	return out
}

func (c *Catalog) Archived() bool { c.mu.RLock(); defer c.mu.RUnlock(); return c.archived }

func (c *Catalog) HasRepository(repositoryID kernel.RepositoryID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.repositories[string(repositoryID)]
	return ok
}

func (c *Catalog) RegisterRepository(repositoryID kernel.RepositoryID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return err
	}
	id := string(repositoryID)
	if id == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository id is required")
	}
	if c.store != nil {
		if repo, ok := c.store.Get(repositoryID); ok && repo.Archived() {
			return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", id)
		}
	}
	next := c.dumpState()
	if _, ok := c.repositories[id]; !ok {
		next.Repositories = append(next.Repositories, id)
	}
	// Unchanged state still crosses the authority check; persistence does not
	// create another commit when this handle remains current.
	return c.persist(next, "register "+id)
}

func (c *Catalog) UnregisterRepository(repositoryID kernel.RepositoryID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return err
	}
	id := string(repositoryID)
	if id == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository id is required")
	}
	if _, ok := c.repositories[id]; !ok {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s is not registered in this catalog", id)
	}
	next := c.dumpState()
	filtered := next.Repositories[:0]
	for _, repo := range next.Repositories {
		if repo != id {
			filtered = append(filtered, repo)
		}
	}
	next.Repositories = filtered
	return c.persist(next, "unregister "+id)
}

func (c *Catalog) RetireWorkspace(workspaceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return err
	}
	if _, ok := c.workspaces[workspaceID]; !ok {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is not defined in this catalog", workspaceID)
	}
	next := c.dumpState()
	for i := range next.Workspaces {
		if next.Workspaces[i].WorkspaceID == workspaceID {
			next.Workspaces[i].Retired = true
		}
	}
	return c.persist(next, "retire-workspace "+workspaceID)
}

func (c *Catalog) Archive() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.dumpState()
	next.Archived = true
	return c.persist(next, "archive-catalog")
}

func (c *Catalog) ensureWritable() error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a Catalog read view cannot persist changes")
	}
	if c.archived {
		return kernel.Fail(kernel.ErrCatalogArchived, "catalog %s is archived", c.registry.CatalogID())
	}
	return nil
}

func (c *Catalog) requireRepository(repositoryID kernel.RepositoryID) error {
	if !c.HasRepository(repositoryID) {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s is not registered in this catalog", repositoryID)
	}
	return nil
}

// Require returns a mounted member Snapshot. Catalog still does not read object_id,
// so composition, checkout and path routing stop at this capability.
func (c *Catalog) Require(repositoryID kernel.RepositoryID) (snapshot.Store, error) {
	if c.store == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog has no store")
	}
	return c.store.Require(repositoryID, kernel.ErrUsageInvalid)
}
