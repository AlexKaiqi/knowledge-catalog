package dataset

// service.Log is Snapshot history of the registry files (dataset-define /
// register / dataset-retire). It is not Repository.LOG.

type Commit struct {
	Commit    string `json:"commit"`
	Author    string `json:"author,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	RuleID    string `json:"ruleId,omitempty"`
}

type LogQuery struct {
	Dataset string
	Limit   int
}

type History struct {
	RepositoryID string   `json:"repositoryId,omitempty"`
	Commits      []Commit `json:"commits"`
}

func (c *service) Log(query LogQuery) (History, error) {
	path := ""
	if query.Dataset != "" {
		path = DatasetYAML(query.Dataset)
	}
	history := History{RepositoryID: c.registry.AuthorityID(), Commits: []Commit{}}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	commits, err := c.registry.history(c.accepted().head, limit, path)
	if err != nil {
		return history, err
	}
	if commits != nil {
		history.Commits = commits
	}
	return history, nil
}

// history walks immutable commits from the caller's accepted head, so a read
// view lists the registry version it was frozen at and never waits for a write.
func (g *registry) history(head string, limit int, path string) ([]Commit, error) {
	return g.snapshotHistory(head, limit, path)
}
