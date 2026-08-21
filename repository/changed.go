package repository

import "kc/kernel"

func UniqueObjectIDs(ops []Operation) []kernel.ObjectID {
	seen := map[kernel.ObjectID]struct{}{}
	var out []kernel.ObjectID
	for _, op := range ops {
		id := op.Address.ObjectID
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ChangedObjectIDs lists objects that differ between two pinned commits.
// FileGit uses diff-tree; other adapters compare List digests.
func ChangedObjectIDs(repo Repository, from, to kernel.CommitID) ([]kernel.ObjectID, error) {
	if to == "" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "to commit is required")
	}
	if from == "" || from == to {
		return objectIDsAt(repo, to)
	}
	if fg, ok := repo.(interface {
		FastChangedObjectIDs(from, to kernel.CommitID) ([]kernel.ObjectID, error)
	}); ok {
		ids, err := fg.FastChangedObjectIDs(from, to)
		if err == nil {
			return ids, nil
		}
	}
	return changedByList(repo, from, to)
}

func objectIDsAt(repo Repository, commit kernel.CommitID) ([]kernel.ObjectID, error) {
	listed, err := repo.List(commit)
	if err != nil {
		return nil, err
	}
	out := make([]kernel.ObjectID, 0, len(listed))
	for _, value := range listed {
		out = append(out, value.Address.ObjectID)
	}
	return out, nil
}

func changedByList(repo Repository, from, to kernel.CommitID) ([]kernel.ObjectID, error) {
	older, err := indexDigests(repo, from)
	if err != nil {
		return nil, err
	}
	newer, err := indexDigests(repo, to)
	if err != nil {
		return nil, err
	}
	seen := map[kernel.ObjectID]struct{}{}
	for id, digest := range newer {
		if older[id] != digest {
			seen[id] = struct{}{}
		}
	}
	for id := range older {
		if _, ok := newer[id]; !ok {
			seen[id] = struct{}{}
		}
	}
	out := make([]kernel.ObjectID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

func indexDigests(repo Repository, commit kernel.CommitID) (map[kernel.ObjectID]kernel.Digest, error) {
	listed, err := repo.List(commit)
	if err != nil {
		return nil, err
	}
	out := map[kernel.ObjectID]kernel.Digest{}
	for _, value := range listed {
		out[value.Address.ObjectID] = kernel.CanonicalDigest(value.Value)
	}
	return out, nil
}
