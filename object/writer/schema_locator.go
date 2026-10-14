package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// Schema and Bound directories are derived from the same final unit set and
// versioned together. Ordinary object updates do not read either directory.
func updateSchemaLocator(tree snapshot.TreeStore, commit kernel.CommitID, idx *repofile.Tree, operations []objectmodel.Operation, toWrite map[string]string) error {
	changed := []objectmodel.ObjectID{}
	for _, id := range changedObjectIDs(operations) {
		if objectmodel.IsSchemaObject(id) {
			changed = append(changed, id)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	schemas, _, err := readSchemaCoordinates(tree, repofile.LocatorSchemaIndexPath, commit)
	if err != nil {
		return err
	}
	bindings, found, err := readSchemaCoordinates(tree, repofile.LocatorBindingIndexPath, commit)
	if err != nil {
		return err
	}
	if !found && len(schemas) != 0 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"knowledge locator has schemas but lacks Binding coordinates; explicit locator rebuild is required")
	}
	final := repofile.BuildLocatorManifest(idx)
	finalBindings := map[objectmodel.ObjectID]struct{}{}
	for _, id := range final.BindingSchemas {
		finalBindings[id] = struct{}{}
	}
	for _, id := range changed {
		delete(schemas, id)
		delete(bindings, id)
		if len(idx.ObjectUnits(id)) == 0 {
			continue
		}
		schemas[id] = struct{}{}
		if _, bound := finalBindings[id]; bound {
			bindings[id] = struct{}{}
		}
	}
	for file, ids := range map[string]map[objectmodel.ObjectID]struct{}{
		repofile.LocatorSchemaIndexPath: schemas, repofile.LocatorBindingIndexPath: bindings,
	} {
		values := make([]objectmodel.ObjectID, 0, len(ids))
		for id := range ids {
			values = append(values, id)
		}
		encoded, err := repofile.EncodeSchemaIndex(values)
		if err != nil {
			return err
		}
		toWrite[file] = string(encoded)
	}
	return nil
}

func readSchemaCoordinates(tree snapshot.TreeReader, file string, commit kernel.CommitID) (map[objectmodel.ObjectID]struct{}, bool, error) {
	present := map[objectmodel.ObjectID]struct{}{}
	raw, err := tree.ReadFile(file, commit)
	if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
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
