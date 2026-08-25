package repofile

import (
	"kc/kernel"
	"kc/knowledge"
)

// Ingest adds one parsed unit at relPath. Duplicate Address or blob/aspect mix fails.
func Ingest(idx *Tree, parsed *Unit, relPath string) error {
	if parsed == nil {
		return nil
	}
	if parsed.declarationErr != nil {
		return parsed.declarationErr
	}
	key := knowledge.AddressKey(parsed.Address)
	if _, ok := idx.Units[key]; ok {
		return kernel.Fail(kernel.ErrObjectIDConflict, "duplicate address %s", key)
	}
	siblings := idx.ObjectUnits(parsed.ObjectID)
	incomingBlob := knowledge.IsEntityBlob(parsed.Address)
	siblingBlob, siblingAspect := false, false
	for _, u := range siblings {
		if knowledge.IsEntityBlob(u.Address) {
			siblingBlob = true
		} else {
			siblingAspect = true
		}
	}
	if (incomingBlob && siblingAspect) || (!incomingBlob && siblingBlob) {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", parsed.ObjectID)
	}
	parsed.Path = relPath
	parsed.Digest = kernel.CanonicalDigest(parsed.Value)
	idx.Upsert(*parsed)
	return nil
}
