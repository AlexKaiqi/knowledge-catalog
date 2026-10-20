package format

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// ReadAddressUnit reads the one maintained unit selected by derived coordinates.
// It never infers an Address from a filename or loads sibling unit bodies.
func ReadAddressUnit(tree snapshot.TreeReader, address objectmodel.Address, commit kernel.CommitID) (Unit, bool, error) {
	locations, err := ReadObjectUnitLocations(tree, address.ObjectID, commit)
	if err != nil {
		return Unit{}, false, err
	}
	for _, location := range locations {
		if objectmodel.AddressKey(location.Address) != objectmodel.AddressKey(address) {
			continue
		}
		raw, err := tree.ReadFile(location.Path, commit)
		if err != nil {
			return Unit{}, false, err
		}
		unit := Parse(string(raw))
		if unit == nil || objectmodel.AddressKey(unit.Address) != objectmodel.AddressKey(address) {
			return Unit{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "unit coordinates for %s contain a mismatched unit", objectmodel.AddressLabel(address))
		}
		idx := NewTree()
		if err := Ingest(idx, unit, location.Path); err != nil {
			return Unit{}, false, err
		}
		return idx.Units[objectmodel.AddressKey(address)], true, nil
	}
	return Unit{}, false, nil
}
