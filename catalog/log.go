package catalog

import "kc/internal/gitdir"

// Catalog.Log is git history of the registry files (define-workspace /
// register / retire-workspace). It is not Repository.LOG.

type CatalogCommit = gitdir.LogEntry

type CatalogLogQuery struct {
	Workspace string
	Limit     int
}

type CatalogHistory struct {
	RepositoryID string          `json:"repositoryId,omitempty"`
	Commits      []CatalogCommit `json:"commits"`
}

func (c *Catalog) Log(query CatalogLogQuery) CatalogHistory {
	path := ""
	if query.Workspace != "" {
		path = WorkspaceYAML(query.Workspace)
	}
	history := CatalogHistory{RepositoryID: c.registry.CatalogID(), Commits: []CatalogCommit{}}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	commits, err := c.registry.history(limit, path)
	if err != nil || commits == nil {
		return history
	}
	history.Commits = commits
	return history
}

func (g *Registry) history(limit int, path string) ([]CatalogCommit, error) {
	return g.dir.Log(limit, path)
}
