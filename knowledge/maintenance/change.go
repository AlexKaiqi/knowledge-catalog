package maintenance

import (
	"kc/kernel"
	"kc/knowledge"
)

func ChangedObjectIDs(repo knowledge.Repository, from, to kernel.CommitID) ([]knowledge.ObjectID, error) {
	if to == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "to commit is required")
	}
	if fast, ok := repo.(knowledge.FastChanges); ok && from != "" && from != to {
		// A declared native capability is part of this provider's contract.
		// Its failure is not permission to switch to two repository-wide scans:
		// callers must see the real availability/basis failure and decide how
		// to recover explicitly.
		return fast.FastChangedObjectIDs(from, to)
	}
	scanner, err := RequireScanner(repo)
	if err != nil {
		return nil, err
	}
	if from == "" || from == to {
		return objectIDsAt(scanner, to)
	}
	older, err := digestIndex(scanner, from)
	if err != nil {
		return nil, err
	}
	newer, err := digestIndex(scanner, to)
	if err != nil {
		return nil, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
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
	out := make([]knowledge.ObjectID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

func objectIDsAt(scanner SnapshotScanner, commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	out := []knowledge.ObjectID{}
	err := WalkSnapshot(scanner, commit, func(value knowledge.KnowledgeValue) error {
		out = append(out, value.Address.ObjectID)
		return nil
	})
	return out, err
}

func digestIndex(scanner SnapshotScanner, commit kernel.CommitID) (map[knowledge.ObjectID]kernel.Digest, error) {
	out := map[knowledge.ObjectID]kernel.Digest{}
	err := WalkSnapshot(scanner, commit, func(value knowledge.KnowledgeValue) error {
		out[value.Address.ObjectID] = kernel.CanonicalDigest(value.Value)
		return nil
	})
	return out, err
}
