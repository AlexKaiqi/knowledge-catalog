package local

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

var _ repository.FastChanges = (*FileGitRepository)(nil)

func (r *FileGitRepository) FastChangedObjectIDs(from, to kernel.CommitID) ([]kernel.ObjectID, error) {
	raw, err := git(r.rootDir, "diff-tree", "--no-commit-id", "--name-only", "-r", string(from), string(to))
	if err != nil {
		return nil, err
	}
	seen := map[kernel.ObjectID]struct{}{}
	for _, path := range splitNonEmpty(raw) {
		for _, commit := range []kernel.CommitID{to, from} {
			body, err := git(r.rootDir, "show", string(commit)+":"+path)
			if err != nil {
				continue
			}
			obj := repofile.Parse(body)
			if obj == nil {
				continue
			}
			seen[obj.ObjectID] = struct{}{}
			break
		}
	}
	out := make([]kernel.ObjectID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}
