package gitea

import (
	"net/http"
	"net/url"
	"sort"

	"kc/kernel"
)

type compareResult struct {
	TotalCommits int `json:"total_commits"`
	Commits      []struct {
		Files []changedFile `json:"files"`
	} `json:"commits"`
}

type changedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
}

// ChangedPaths uses Gitea's commit comparison rather than loading both
// repository trees. Its transfer cost is bounded by the changed-file set and
// the client's response limit, not by the total number of repository files.
func (r *Repository) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	if from == "" || to == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "changed paths require from and to commits")
	}
	if from == to {
		return []string{}, nil
	}
	path := "/repos/" + url.PathEscape(r.ep.Owner) + "/" + url.PathEscape(r.ep.Name) +
		"/compare/" + url.PathEscape(string(from)+"..."+string(to)) + "?files=true&limit=1000"
	var comparison compareResult
	if _, _, err := r.cli.do(http.MethodGet, path, nil, &comparison); err != nil {
		return nil, err
	}
	if comparison.TotalCommits > len(comparison.Commits) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"gitea compare returned %d of %d commits", len(comparison.Commits), comparison.TotalCommits)
	}
	seen := map[string]struct{}{}
	for _, commit := range comparison.Commits {
		for _, file := range commit.Files {
			if file.Filename != "" {
				seen[file.Filename] = struct{}{}
			}
			if file.PreviousFilename != "" {
				seen[file.PreviousFilename] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}
