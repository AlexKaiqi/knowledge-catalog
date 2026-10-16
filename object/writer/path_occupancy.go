package writer

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// Changed object locators are an intentionally bounded working set. Check the
// exact destination files as well so a PUT for a new identity cannot overwrite
// a unit belonging to an object outside that working set. Removing an old unit
// is an explicit decision in the same batch, never inferred from a reused path.
func requireAvailableUnitPaths(tree snapshot.TreeReader, base kernel.CommitID, final *repofile.Tree, operations []objectmodel.Operation, writes map[string]string) error {
	owners := map[string]objectmodel.Address{}
	for _, unit := range final.Units {
		if previous, exists := owners[unit.Path]; exists && objectmodel.AddressKey(previous) != objectmodel.AddressKey(unit.Address) {
			return kernel.Fail(kernel.ErrObjectIDConflict,
				"path %q is assigned to both %s and %s; choose distinct paths for the maintained units", unit.Path, previous.ObjectID, unit.Address.ObjectID)
		}
		owners[unit.Path] = unit.Address
	}
	removed := map[string]bool{}
	for _, operation := range operations {
		if operation.Op == objectmodel.OpRemove {
			removed[objectmodel.AddressKey(operation.Address)] = true
		}
	}
	paths := make([]string, 0, len(writes))
	for path := range writes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := requireAvailableUnitPath(tree, base, path, owners[path], removed); err != nil {
			return err
		}
	}
	return nil
}

func requireAvailableUnitPath(tree snapshot.TreeReader, base kernel.CommitID, path string, next objectmodel.Address, removed map[string]bool) error {
	raw, err := tree.ReadFile(path, base)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil
	}
	if err != nil {
		return err
	}
	previous := repofile.Parse(string(raw))
	if previous == nil {
		// An unindexed pure Snapshot file is occupied storage too. Object
		// publication cannot infer permission to replace its unrelated bytes.
		return kernel.Fail(kernel.ErrObjectIDConflict,
			"path %q already contains a file without a declared object identity; choose another path for %s", path, next.ObjectID)
	}
	key := objectmodel.AddressKey(previous.Address)
	rootKey := objectmodel.AddressKey(objectmodel.Address{ObjectID: previous.ObjectID})
	if key == objectmodel.AddressKey(next) || removed[key] || removed[rootKey] {
		return nil
	}
	return kernel.Fail(kernel.ErrObjectIDConflict,
		"path %q belongs to object %s (aspect %q, member %q); explicitly REMOVE that unit before publishing %s at this path",
		path, previous.ObjectID, previous.Address.AspectName, previous.Address.MemberKey, next.ObjectID)
}
