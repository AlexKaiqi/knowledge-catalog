package catalog

import "kc/kernel"

type catalogMeta struct {
	ID       string `json:"id,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

func (c *Catalog) Repositories() []string {
	out := make([]string, 0, len(c.repositories))
	for id := range c.repositories {
		out = append(out, id)
	}
	return out
}

func (c *Catalog) Archived() bool { return c.archived }

func (c *Catalog) HasRepository(repositoryID kernel.RepositoryID) bool {
	_, ok := c.repositories[string(repositoryID)]
	return ok
}

func (c *Catalog) RegisterRepository(repositoryID kernel.RepositoryID) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	id := string(repositoryID)
	if id == "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "repository id is required")
	}
	if repo, ok := c.store.Get(repositoryID); ok && repo.Archived() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", id)
	}
	if _, ok := c.repositories[id]; ok {
		return nil
	}
	c.repositories[id] = struct{}{}
	return c.persist("register " + id)
}

func (c *Catalog) RetireDefinition(viewID string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	def, err := c.View(viewID)
	if err != nil {
		return err
	}
	if def.Retired {
		return nil
	}
	def.Retired = true
	c.views[viewID] = def
	return c.persist("retire-view " + viewID)
}

func (c *Catalog) Archive() error {
	if c.archived {
		return nil
	}
	c.archived = true
	return c.persist("archive-catalog")
}

func (c *Catalog) ensureWritable() error {
	if c.archived {
		return kernel.Fail(kernel.ErrCatalogArchived, "catalog %s is archived", c.registry.CatalogID())
	}
	return nil
}

func (c *Catalog) requireRepository(repositoryID kernel.RepositoryID) error {
	if !c.HasRepository(repositoryID) {
		return kernel.Fail(kernel.ErrViewGenerationInvalid, "repository %s is not registered in this catalog", repositoryID)
	}
	return nil
}
