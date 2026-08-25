package knowledge

import "kc/kernel"

func UniqueObjectIDs(ops []Operation) []ObjectID {
	seen := map[ObjectID]struct{}{}
	var out []ObjectID
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

type FastChanges interface {
	FastChangedObjectIDs(from, to kernel.CommitID) ([]ObjectID, error)
}

func ChangedObjectIDs(repo Repository, from, to kernel.CommitID) ([]ObjectID, error) {
	if to == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "to commit is required")
	}
	if from == "" || from == to {
		return objectIDsAt(repo, to)
	}
	if fast, ok := repo.(FastChanges); ok {
		if ids, err := fast.FastChangedObjectIDs(from, to); err == nil {
			return ids, nil
		}
	}
	return changedByList(repo, from, to)
}

func objectIDsAt(repo Repository, commit kernel.CommitID) ([]ObjectID, error) {
	listed, err := repo.List(commit)
	if err != nil {
		return nil, err
	}
	out := make([]ObjectID, 0, len(listed))
	for _, value := range listed {
		out = append(out, value.Address.ObjectID)
	}
	return out, nil
}

func changedByList(repo Repository, from, to kernel.CommitID) ([]ObjectID, error) {
	older, err := indexDigests(repo, from)
	if err != nil {
		return nil, err
	}
	newer, err := indexDigests(repo, to)
	if err != nil {
		return nil, err
	}
	seen := map[ObjectID]struct{}{}
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
	out := make([]ObjectID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

func indexDigests(repo Repository, commit kernel.CommitID) (map[ObjectID]kernel.Digest, error) {
	listed, err := repo.List(commit)
	if err != nil {
		return nil, err
	}
	out := map[ObjectID]kernel.Digest{}
	for _, value := range listed {
		out[value.Address.ObjectID] = kernel.CanonicalDigest(value.Value)
	}
	return out, nil
}
