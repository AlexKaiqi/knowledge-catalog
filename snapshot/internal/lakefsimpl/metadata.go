package lakefsimpl

import (
	"net/http"
	"net/url"

	"kc/kernel"
	"kc/snapshot/internal/storecontract"
)

// ReadCommitMetadata reads the immutable GetCommit response. The lakeFS
// committer remains the service identity; an explicit caller stamp is stored
// separately in metadata and takes precedence for Snapshot provenance.
func (r *Repository) ReadCommitMetadata(commit kernel.CommitID) (storecontract.CommitMetadata, error) {
	if commit == "" {
		return storecontract.CommitMetadata{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit is required")
	}
	var out struct {
		ID        string            `json:"id"`
		Committer string            `json:"committer"`
		Message   string            `json:"message"`
		Metadata  map[string]string `json:"metadata"`
	}
	status, _, err := r.client.doJSON(http.MethodGet,
		r.client.repositoryPath("commits/"+url.PathEscape(string(commit))), nil, &out)
	if status == http.StatusNotFound {
		return storecontract.CommitMetadata{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if err != nil {
		return storecontract.CommitMetadata{}, err
	}
	if out.ID != string(commit) {
		return storecontract.CommitMetadata{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs returned a different commit coordinate")
	}
	author := out.Metadata["kc.author"]
	if author == "" {
		author = out.Committer
	}
	r.rememberCommit(commit)
	return storecontract.CommitMetadata{
		Commit: commit, Author: author, Message: out.Message,
		RequestID: out.Metadata["kc.command_id"], RuleID: out.Metadata["kc.rule_id"],
	}, nil
}
