package lakefsimpl

import (
	"net/http"
	"net/url"

	"kc/kernel"
)

// CommitHistory honors the caller's total limit independently of lakeFS's
// physical page limit, always along the immutable first-parent chain.
func (r *Repository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	if !r.HasCommit(commit) {
		if err := r.client.canceled(); err != nil {
			return nil, err
		}
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = pageAmount(limit)
	}
	var out []kernel.CommitID
	continuation := ""
	offsets := map[string]bool{}
	commits := map[kernel.CommitID]bool{}
	for len(out) < limit {
		query := url.Values{"first_parent": {"true"}, "limit": {"true"}}
		addPage(query, limit-len(out), continuation)
		var page struct {
			Pagination pagination `json:"pagination"`
			Results    []struct {
				ID kernel.CommitID `json:"id"`
			} `json:"results"`
		}
		endpoint := r.client.repositoryPath("refs/"+url.PathEscape(string(commit))+"/commits") + "?" + query.Encode()
		if _, _, err := r.client.doJSON(http.MethodGet, endpoint, nil, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Results {
			if item.ID == "" || commits[item.ID] {
				return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs history returned an invalid commit")
			}
			commits[item.ID] = true
			out = append(out, item.ID)
			if len(out) == limit {
				return out, nil
			}
		}
		if !page.Pagination.HasMore {
			return out, nil
		}
		next := page.Pagination.NextOffset
		if len(page.Results) == 0 || next == "" || offsets[next] {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs history returned an invalid continuation")
		}
		offsets[next] = true
		continuation = next
	}
	return out, nil
}
