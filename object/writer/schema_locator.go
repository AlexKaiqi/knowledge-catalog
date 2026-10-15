package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// The Schema directory and the binding unit coordinates are derived from the
// same final unit set and versioned together. A commit reads them only when it
// changes a Schema or a binding unit; ordinary object updates read neither.
func updateSchemaLocator(tree snapshot.TreeStore, commit kernel.CommitID, before []repofile.BindingCoordinate, idx *repofile.Tree, operations []objectmodel.Operation, toWrite map[string]string) error {
	touched := map[objectmodel.ObjectID]struct{}{}
	changedSchemas := []objectmodel.ObjectID{}
	for _, id := range changedObjectIDs(operations) {
		touched[id] = struct{}{}
		if objectmodel.IsSchemaObject(id) {
			changedSchemas = append(changedSchemas, id)
		}
	}
	after := []repofile.BindingCoordinate{}
	for _, coordinate := range repofile.BindingCoordinates(idx) {
		if _, ok := touched[coordinate.Address.ObjectID]; ok {
			after = append(after, coordinate)
		}
	}
	if len(changedSchemas) == 0 && len(before) == 0 && len(after) == 0 {
		return nil
	}
	schemas, _, err := readSchemaCoordinates(tree, repofile.LocatorSchemaIndexPath, commit)
	if err != nil {
		return err
	}
	bindings, found, err := readBindingCoordinates(tree, commit)
	if err != nil {
		return err
	}
	if !found && len(schemas) != 0 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"object locator has schemas but lacks binding unit coordinates; explicit locator rebuild is required")
	}
	if len(changedSchemas) > 0 {
		for _, id := range changedSchemas {
			delete(schemas, id)
			if len(idx.ObjectUnits(id)) > 0 {
				schemas[id] = struct{}{}
			}
		}
		values := make([]objectmodel.ObjectID, 0, len(schemas))
		for id := range schemas {
			values = append(values, id)
		}
		encoded, err := repofile.EncodeSchemaIndex(values)
		if err != nil {
			return err
		}
		toWrite[repofile.LocatorSchemaIndexPath] = string(encoded)
	}
	final := after
	for _, coordinate := range bindings {
		if _, ok := touched[coordinate.Address.ObjectID]; !ok {
			final = append(final, coordinate)
		}
	}
	encoded, err := repofile.EncodeBindingIndex(final)
	if err != nil {
		return err
	}
	toWrite[repofile.LocatorBindingIndexPath] = string(encoded)
	return nil
}

func readBindingCoordinates(tree snapshot.TreeReader, commit kernel.CommitID) ([]repofile.BindingCoordinate, bool, error) {
	raw, err := tree.ReadFile(repofile.LocatorBindingIndexPath, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	coordinates, err := repofile.DecodeBindingIndex(raw)
	if err != nil {
		return nil, false, kernel.Fail(kernel.ErrPreconditionFailed,
			"invalid binding unit coordinates at %s: %v", commit, err)
	}
	return coordinates, true, nil
}

func readSchemaCoordinates(tree snapshot.TreeReader, file string, commit kernel.CommitID) (map[objectmodel.ObjectID]struct{}, bool, error) {
	present := map[objectmodel.ObjectID]struct{}{}
	raw, err := tree.ReadFile(file, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return present, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	ids, err := repofile.DecodeSchemaIndex(raw)
	if err != nil {
		return nil, false, kernel.Fail(kernel.ErrPreconditionFailed,
			"invalid schema coordinates %s at %s: %v", file, commit, err)
	}
	for _, id := range ids {
		present[id] = struct{}{}
	}
	return present, true, nil
}
