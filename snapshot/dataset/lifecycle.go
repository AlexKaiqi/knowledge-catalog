package dataset

import (
	"slices"

	"kc/kernel"
	"kc/snapshot"
)

type registryMeta struct {
	AuthorityID  string   `json:"authorityId"`
	Repositories []string `json:"repositories"`
}

func (c *service) Repositories() []string {
	return c.accepted().repositoriesList()
}

func (s *acceptedState) repositoriesList() []string {
	out := make([]string, 0, len(s.repositories))
	for id := range s.repositories {
		out = append(out, id)
	}
	return out
}

func (c *service) HasRepository(repositoryID kernel.RepositoryID) bool {
	_, ok := c.accepted().repositories[string(repositoryID)]
	return ok
}

func (c *service) RegisterRepository(repositoryID kernel.RepositoryID) error {
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
	// Unchanged state still crosses the authority check; persistence does not
	// create another commit when this handle remains current.
	return c.commit("register "+id, func(s *acceptedState) (State, error) {
		next := s.dump(c.registry.AuthorityID())
		if _, ok := s.repositories[id]; !ok {
			next.Repositories = append(next.Repositories, id)
		}
		return next, nil
	})
}

func (c *service) UnregisterRepository(repositoryID kernel.RepositoryID) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	id := string(repositoryID)
	if id == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository id is required")
	}
	return c.commit("unregister "+id, func(s *acceptedState) (State, error) {
		if _, ok := s.repositories[id]; !ok {
			return State{}, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not registered in this service", id)
		}
		next := s.dump(c.registry.AuthorityID())
		next.Repositories = slices.DeleteFunc(next.Repositories, func(repo string) bool { return repo == id })
		return next, nil
	})
}

func (c *service) RetireDataset(setID string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	return c.commit("dataset-retire "+setID, func(s *acceptedState) (State, error) {
		if _, ok := s.datasets[setID]; !ok {
			return State{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is not defined in this service", setID)
		}
		next := s.dump(c.registry.AuthorityID())
		for i := range next.Datasets {
			if next.Datasets[i].SetID == setID {
				next.Datasets[i].Retired = true
			}
		}
		return next, nil
	})
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
