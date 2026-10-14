package dataset

import (
	"kc/kernel"
	"kc/snapshot"
)

type registryMeta struct {
	AuthorityID  string   `json:"authorityId"`
	Repositories []string `json:"repositories"`
}

func (c *service) Repositories() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.repositoriesList()
}

func (c *service) repositoriesList() []string {
	out := make([]string, 0, len(c.repositories))
	for id := range c.repositories {
		out = append(out, id)
	}
	return out
}

func (c *service) HasRepository(repositoryID kernel.RepositoryID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.repositories[string(repositoryID)]
	return ok
}

func (c *service) RegisterRepository(repositoryID kernel.RepositoryID) error {
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

func (c *service) UnregisterRepository(repositoryID kernel.RepositoryID) error {
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
		return kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not registered in this service", id)
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

func (c *service) RetireDataset(setID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return err
	}
	if _, ok := c.datasets[setID]; !ok {
		return kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is not defined in this service", setID)
	}
	next := c.dumpState()
	for i := range next.Datasets {
		if next.Datasets[i].SetID == setID {
			next.Datasets[i].Retired = true
		}
	}
	return c.persist(next, "dataset-retire "+setID)
}

func (c *service) ensureWritable() error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a service read view cannot persist changes")
	}
	return nil
}

func (c *service) requireRepository(repositoryID kernel.RepositoryID) error {
	if !c.HasRepository(repositoryID) {
		return kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not registered in this service", repositoryID)
	}
	return nil
}

// Require returns a mounted member Snapshot. service still does not read object_id,
// so composition, checkout and path routing stop at this capability.
func (c *service) Require(repositoryID kernel.RepositoryID) (snapshot.Store, error) {
	if c.store == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "service has no store")
	}
	return c.store.Require(repositoryID, kernel.ErrUsageInvalid)
}
