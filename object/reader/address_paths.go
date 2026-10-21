package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

func (r *treeRepository) addressAtPaths(address objectmodel.Address, commit kernel.CommitID, paths []string) (repofile.Unit, error) {
	unit, found, err := r.addressUnit(address, commit, paths)
	if err != nil {
		return repofile.Unit{}, err
	}
	if !found {
		return repofile.Unit{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "address is outside selected file paths")
	}
	return unit, nil
}

func (r *treeRepository) addressUnit(address objectmodel.Address, commit kernel.CommitID, paths []string) (repofile.Unit, bool, error) {
	if err := objectmodel.AssertWritable(address); err != nil {
		return repofile.Unit{}, false, err
	}
	locations, err := repofile.ReadObjectUnitLocations(r.tree, address.ObjectID, commit)
	if err != nil {
		return repofile.Unit{}, false, err
	}
	for _, location := range locations {
		if objectmodel.AddressKey(location.Address) != objectmodel.AddressKey(address) {
			continue
		}
		if paths != nil && !containsPath(paths, location.Path) {
			return repofile.Unit{}, false, nil
		}
		units, err := r.readLocatedUnits([]repofile.ObjectUnitLocation{location}, address.ObjectID, commit)
		if err != nil {
			return repofile.Unit{}, false, err
		}
		return units[0], true, nil
	}
	return repofile.Unit{}, false, nil
}

func (r *treeRepository) ReadAddressAtPaths(address objectmodel.Address, commit kernel.CommitID, paths []string) (objectmodel.ObjectValue, error) {
	unit, err := r.addressAtPaths(address, commit, paths)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return objectmodel.ObjectValue{ObjectRef: objectmodel.ObjectRef{Repository: r.ID(), Object: address.ObjectID}, Repository: r.ID(), Commit: commit, Address: unit.Address, Value: unit.Value, Provenance: unit.Provenance, Declarations: []objectmodel.UnitDeclaration{repofile.DeclarationOf(unit)}}, nil
}

func (r *treeRepository) ResolveAddressAtPaths(address objectmodel.Address, commit kernel.CommitID, paths []string) (objectmodel.Resolution, error) {
	unit, err := r.addressAtPaths(address, commit, paths)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	hint := unit.PathHint
	if hint == "" {
		hint = unit.Path
	}
	return objectmodel.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID, Address: unit.Address, PathHint: hint, Digest: objectmodel.MaintenanceDigest(unit.Digest, unit.SchemaRef, unit.ValueSource), DeclarationDigest: objectmodel.DeclarationDigest(unit.SchemaRef, unit.ValueSource), SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource, Status: objectmodel.StatusResolved}, nil
}
