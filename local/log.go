package local

import "kc/kernel"
import "kc/repository"

func (r *FileGitRepository) Log(objectID kernel.ObjectID, commitID kernel.CommitID, limit int) ([]repository.ObjectRevision, error) {
	if !r.HasCommit(commitID) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	if limit <= 0 {
		limit = 50
	}
	raw, err := git(r.rootDir, "log", "--format=%H", string(commitID))
	if err != nil {
		return nil, err
	}
	var out []repository.ObjectRevision
	previous := ""
	var introducing *repository.ObjectRevision
	for _, hash := range splitNonEmpty(raw) {
		resolution, err := r.Resolve(objectID, kernel.CommitID(hash))
		if err != nil {
			return nil, err
		}
		if resolution.Status == repository.StatusUnresolved {
			if introducing != nil {
				out = append(out, *introducing)
			}
			break
		}
		key := string(resolution.Status) + ":" + string(resolution.Digest) + ":" + string(resolution.DeclarationDigest)
		rev := repository.ObjectRevision{Commit: kernel.CommitID(hash), Status: resolution.Status, Digest: resolution.Digest, DeclarationDigest: resolution.DeclarationDigest}
		if key == previous {
			introducing = &rev
			continue
		}
		if introducing != nil {
			out = append(out, *introducing)
			if len(out) >= limit {
				return out, nil
			}
		}
		previous = key
		copyRev := rev
		introducing = &copyRev
	}
	if introducing != nil && len(out) < limit {
		out = append(out, *introducing)
	}
	return out, nil
}

func (r *FileGitRepository) Diff(objectID kernel.ObjectID, from, to kernel.CommitID) (repository.ObjectDiff, error) {
	readSide := func(commit kernel.CommitID) (*repository.KnowledgeValue, error) {
		resolution, err := r.Resolve(objectID, commit)
		if err != nil {
			return nil, err
		}
		if resolution.Status != repository.StatusResolved {
			return nil, nil
		}
		kv, err := r.Read(objectID, commit)
		if err != nil {
			return nil, err
		}
		return &kv, nil
	}
	left, err := readSide(from)
	if err != nil {
		return repository.ObjectDiff{}, err
	}
	right, err := readSide(to)
	if err != nil {
		return repository.ObjectDiff{}, err
	}
	return repository.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
}
