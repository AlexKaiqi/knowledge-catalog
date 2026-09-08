package integrationruntime

import (
	"context"
	"kc/client"
	"kc/kernel"
	"kc/knowledge"
)

// ClientWriter implements the runtime publication port exclusively with the
// existing authenticated typed Writer API. It never opens local KC state.
type ClientWriter struct{ Client *client.Client }

func (w ClientWriter) Head(ctx context.Context, repository kernel.RepositoryID, ref string) (kernel.CommitID, error) {
	var result struct {
		Repository kernel.RepositoryID `json:"repository"`
		Commit     kernel.CommitID     `json:"commit"`
	}
	err := w.Client.WriterService().Head(ctx, string(repository), ref, client.RequestOptions{}, &result)
	if err != nil {
		return "", err
	}
	if result.Repository != repository || result.Commit == "" {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "Writer returned an invalid repository head")
	}
	return result.Commit, nil
}

func (w ClientWriter) Commit(ctx context.Context, command string, change knowledge.CommitChangeSet) (kernel.CommitID, error) {
	var result struct {
		CommandID string `json:"commandId"`
		Result    struct {
			Repository kernel.RepositoryID `json:"repositoryId"`
			Commit     kernel.CommitID     `json:"commitId"`
		} `json:"result"`
	}
	err := w.Client.WriterService().Commit(ctx, string(change.TargetRepository), client.CommitRequest{CommandID: command, ChangeSet: change}, client.RequestOptions{}, &result)
	if err != nil {
		return "", err
	}
	if result.CommandID != command || result.Result.Repository != change.TargetRepository || result.Result.Commit == "" {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "Writer returned an invalid publication receipt")
	}
	return result.Result.Commit, nil
}
