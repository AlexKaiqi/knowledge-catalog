package filegit

import (
	"kc/kernel"
)

func (r *FileGitRepository) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	raw, err := git(r.rootDir, "diff-tree", "--no-commit-id", "--name-only", "-r", string(from), string(to))
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(raw), nil
}
