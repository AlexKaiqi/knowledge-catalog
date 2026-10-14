package format

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// Ingest adds one parsed unit at relPath. Duplicate Address or blob/aspect mix fails.
func Ingest(idx *Tree, parsed *Unit, relPath string) error {
	if parsed == nil {
		return nil
	}
	if parsed.declarationErr != nil {
		return parsed.declarationErr
	}
	key := objectmodel.AddressKey(parsed.Address)
	if _, ok := idx.Units[key]; ok {
		return kernel.Fail(kernel.ErrObjectIDConflict, "duplicate address %s", key)
	}
	siblings := append(ToObjectUnits(idx.ObjectUnits(parsed.ObjectID)), ToObjectUnits([]Unit{*parsed})...)
	if _, err := objectmodel.AssembleUnits(siblings); err != nil {
		return err
	}
	parsed.Path = relPath
	parsed.Digest = kernel.CanonicalDigest(parsed.Value)
	idx.Upsert(*parsed)
	return nil
}
