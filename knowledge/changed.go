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
