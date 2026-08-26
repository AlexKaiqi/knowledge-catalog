package filegit

import (
	"fmt"

	"kc/kernel"
)

func (r *FileGitRepository) CommitHistory(commitID kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	if !r.HasCommit(commitID) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	if limit <= 0 {
		limit = 1000
	}
	raw, err := git(r.rootDir, "log", "--first-parent", "--format=%H", "-n", fmt.Sprint(limit), string(commitID))
	if err != nil {
		return nil, err
	}
	out := make([]kernel.CommitID, 0, limit)
	for _, hash := range splitNonEmpty(raw) {
		out = append(out, kernel.CommitID(hash))
	}
	return out, nil
}
