package catalog

import (
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

// Serving is the consumer read face on one ResolvedView.
// OpenView resolves selectors once; Read / List / SEARCH / Log hydrate at that map.
// Callers name a View. They do not pass repository, ref, or commit.

type Serving struct {
	catalog *Catalog
	version ViewReadVersion
}

// ObjectLog is object history on one repository at the serving resolution. Not git log.

type ObjectLog struct {
	Repository kernel.RepositoryID         `json:"repository"`
	ObjectID   kernel.ObjectID             `json:"objectId"`
	Commit     kernel.CommitID             `json:"commit"`
	Revisions  []repository.ObjectRevision `json:"revisions"`
}

func (c *Catalog) OpenView(viewID string) (serving *Serving, err error) {
	return c.OpenViewOverlay(viewID, nil)
}

func (c *Catalog) OpenViewOverlay(viewID string, overlay map[kernel.RepositoryID]kernel.CommitID) (serving *Serving, err error) {
	defer func() {
		err = c.note("open-view", map[string]any{"view": viewID}, err)
	}()
	resolved, err := c.ResolveViewOverlay(viewID, overlay)
	if err != nil {
		return nil, err
	}
	return c.openResolved(resolved), nil
}

func (c *Catalog) OpenResolved(resolved ResolvedView) *Serving {
	return c.openResolved(resolved)
}

func (c *Catalog) openResolved(resolved ResolvedView) *Serving {
	return &Serving{
		catalog: c,
		version: ViewReadVersion{Resolved: resolved},
	}
}

func (s *Serving) Version() ViewReadVersion { return s.version }

func (s *Serving) Resolved() ResolvedView { return s.version.Resolved }

func (s *Serving) Read(objectID kernel.ObjectID, selector *repository.AspectSelector) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		value, err := repo.Read(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		v := value.Value
		if selector != nil {
			v = repository.SelectAspects(value.Value, value.Units, selector)
		}
		out = append(out, FederatedValue{
			Repository: repositoryID,
			Commit:     commit,
			ObjectID:   objectID,
			Value:      v,
		})
		return nil
	})
	return out, err
}

func (s *Serving) ReadAddress(address kernel.Address) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		value, err := repo.ReadAddress(address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, FederatedValue{
			Repository: repositoryID,
			Commit:     commit,
			ObjectID:   address.ObjectID,
			Value:      value.Value,
		})
		return nil
	})
	return out, err
}

func (s *Serving) Resolve(objectID kernel.ObjectID) ([]repository.Resolution, error) {
	out := []repository.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		resolution, err := repo.Resolve(objectID, commit)
		if err != nil {
			return err
		}
		if resolution.Status == repository.StatusUnresolved {
			return nil
		}
		out = append(out, resolution)
		return nil
	})
	return out, err
}

func (s *Serving) List() ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		values, err := repo.List(commit)
		if err != nil {
			return err
		}
		for _, value := range values {
			out = append(out, FederatedValue{
				Repository: repositoryID,
				Commit:     commit,
				ObjectID:   value.Address.ObjectID,
				Value:      value.Value,
			})
		}
		return nil
	})
	return out, err
}

func (s *Serving) GetProvenance(objectID kernel.ObjectID) ([]repository.ProvenanceTrace, error) {
	out := []repository.ProvenanceTrace{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		trace, err := repo.GetProvenance(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, trace)
		return nil
	})
	return out, err
}

func (s *Serving) Log(objectID kernel.ObjectID, limit int) ([]ObjectLog, error) {
	out := []ObjectLog{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		revs, err := repo.Log(objectID, commit, limit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		if len(revs) == 0 {
			return nil
		}
		out = append(out, ObjectLog{
			Repository: repositoryID,
			ObjectID:   objectID,
			Commit:     commit,
			Revisions:  revs,
		})
		return nil
	})
	return out, err
}

func (s *Serving) DescribeSchema(objectID kernel.ObjectID) ([]reader.SchemaReport, error) {
	out := []reader.SchemaReport{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		report, err := reader.DescribeRepoSchema(repo, commit, objectID)
		if err != nil {
			return err
		}
		out = append(out, report)
		return nil
	})
	return out, err
}

func (s *Serving) eachRepository(fn func(kernel.RepositoryID, kernel.CommitID, repository.Repository) error) error {
	resolved := s.version.Resolved
	ids := make([]kernel.RepositoryID, 0, len(resolved.Repositories))
	for id := range resolved.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, repositoryID := range ids {
		commit := resolved.Repositories[repositoryID]
		repo, err := s.catalog.store.Require(repositoryID, kernel.ErrTemporaryUnavailable)
		if err != nil {
			return err
		}
		if err := fn(repositoryID, commit, repo); err != nil {
			return err
		}
	}
	return nil
}
