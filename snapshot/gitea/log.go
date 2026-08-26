package gitea

import (
	"net/http"
	"net/url"
	"strconv"

	"kc/kernel"
)

func (r *Repository) CommitHistory(commitID kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	if !r.HasCommit(commitID) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	if limit <= 0 {
		limit = 1000
	}
	var rows []commitRow
	q := "?sha=" + url.QueryEscape(string(commitID)) + "&limit=" + strconv.Itoa(limit)
	if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("commits"+q), nil, &rows); err != nil {
		return nil, err
	}
	out := make([]kernel.CommitID, 0, len(rows))
	for _, row := range rows {
		hash := kernel.CommitID(row.id())
		if hash == "" {
			continue
		}
		out = append(out, hash)
	}
	return out, nil
}
