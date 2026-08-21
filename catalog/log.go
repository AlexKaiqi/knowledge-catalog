package catalog

import (
	"os/exec"
	"strconv"
	"strings"
)

// Catalog.Log is git history of the registry FileGit (define-view / register / retire).
// It is not Repository.LOG.

type CatalogCommit struct {
	Commit    string `json:"commit"`
	Author    string `json:"author,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	RuleID    string `json:"ruleId,omitempty"`
}

type CatalogLogQuery struct {
	View     string
	ObjectID string
	Limit    int
}

type CatalogHistory struct {
	RepositoryID string          `json:"repositoryId,omitempty"`
	Commits      []CatalogCommit `json:"commits"`
}

func (c *Catalog) Log(query CatalogLogQuery) CatalogHistory {
	objectID := query.ObjectID
	if query.View != "" {
		objectID = ViewFile(query.View)
	} else if objectID != "" {
		objectID = registryPath(objectID)
	}
	history := CatalogHistory{RepositoryID: c.registry.CatalogID(), Commits: []CatalogCommit{}}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	commits, err := c.registry.history(limit, objectID)
	if err != nil || commits == nil {
		return history
	}
	history.Commits = commits
	return history
}

func (g *Registry) history(limit int, objectID string) ([]CatalogCommit, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []string{"log", "-" + strconv.Itoa(limit), "--format=%H%x1f%an%x1f%s%x1f%b%x1e"}
	if objectID != "" {
		args = append(args, "--", objectID)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repo.RootDir()
	out, err := cmd.Output()
	if err != nil {
		return []CatalogCommit{}, nil
	}
	raw := strings.Trim(string(out), "\x1e\n ")
	if raw == "" {
		return []CatalogCommit{}, nil
	}
	var commits []CatalogCommit
	for _, rec := range strings.Split(raw, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\x1f", 4)
		if len(parts) < 3 {
			continue
		}
		item := CatalogCommit{Commit: parts[0], Author: parts[1], Message: parts[2]}
		if len(parts) > 3 {
			item.RequestID, item.RuleID = parseCommitTrailers(parts[3])
		}
		commits = append(commits, item)
	}
	return commits, nil
}

func parseCommitTrailers(body string) (requestID, ruleID string) {
	for _, line := range strings.Split(body, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "request-id":
			requestID = strings.TrimSpace(val)
		case "rule-id":
			ruleID = strings.TrimSpace(val)
		}
	}
	return requestID, ruleID
}
