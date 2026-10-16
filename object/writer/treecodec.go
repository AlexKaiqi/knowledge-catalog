package writer

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// applyObjectCommit is the sole ② → ⓪ codec. Snapshot adapters receive
// only literal path changes and never see Address, Aspect, or PUT/REMOVE.
func applyObjectCommit(commandID string, target snapshot.Store, cs objectmodel.ChangeSet, basis snapshot.TreeStore) (_ kernel.CommitID, err error) {
	writeStarted := false
	defer func() {
		if err != nil && !writeStarted {
			err = snapshot.WriteNotApplied(err)
		}
	}()
	if native, ok := target.(objectmodel.ChangeStore); ok {
		// An omitted path retains the existing unit's location. Only the native
		// provider knows whether this is a new unit needing a default path.
		writeStarted = true
		return native.ApplyObjectChange(commandID, cs)
	}
	if _, native := target.(objectmodel.NativeRepository); native {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"native repository %s does not provide typed object writes", target.ID())
	}
	tree, ok := snapshot.TreeStoreOf(target)
	if !ok {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s is mounted as a plain snapshot and cannot accept object changes without tree access", target.ID())
	}
	if basis == nil {
		basis = tree
	}
	if err := requireIncrementalLocatorLayout(basis, cs.BaseCommit); err != nil {
		return "", err
	}

	idx, err := readChangedObjects(basis, cs.BaseCommit, cs.Operations)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	beforeRefs := repofile.BuildLocatorManifest(idx).Referrers
	beforeBindings := repofile.BindingCoordinates(idx)
	if err := repofile.ApplyOperations(idx, cs.Operations, cs.Provenance, toWrite, toDelete); err != nil {
		return "", err
	}
	if err := requireAvailableUnitPaths(basis, cs.BaseCommit, idx, cs.Operations, toWrite); err != nil {
		return "", err
	}
	for _, objectID := range changedObjectIDs(cs.Operations) {
		units := idx.ObjectUnits(objectID)
		locatorPath := repofile.ObjectLocatorPath(objectID)
		if len(units) == 0 {
			toDelete[locatorPath] = struct{}{}
			continue
		}
		locator, err := encodeUnitLocator(objectID, units)
		if err != nil {
			return "", err
		}
		toWrite[locatorPath] = string(locator)
	}
	if err := updateSchemaLocator(basis, cs.BaseCommit, beforeBindings, idx, cs.Operations, toWrite); err != nil {
		return "", err
	}
	if err := updateSchemaReferrerLocators(basis, cs.BaseCommit, beforeRefs, idx, cs.Operations, toWrite, toDelete); err != nil {
		return "", err
	}
	toWrite[repofile.LocatorCompletePath] = repofile.LocatorCompleteBody
	toWrite[repofile.LocatorReferrersCompletePath] = repofile.LocatorReferrersCompleteBody

	changes := make([]snapshot.TreeChange, 0, len(toWrite)+len(toDelete))
	for path := range toDelete {
		if _, replaced := toWrite[path]; !replaced {
			changes = append(changes, snapshot.TreeChange{Path: path, Remove: true})
		}
	}
	for path, content := range toWrite {
		changes = append(changes, snapshot.TreeChange{Path: path, Content: []byte(content)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })

	if cs.BulkIngest {
		bulk, ok := snapshot.BulkTreeIngesterOf(target)
		if !ok {
			return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not provide bulk ingest", target.ID())
		}
		writeStarted = true
		return bulk.ApplyTreeCommitBulk(treeChangeSet(cs, changes))
	}
	writeStarted = true
	return tree.ApplyTreeCommit(treeChangeSet(cs, changes))
}

func encodeUnitLocator(objectID objectmodel.ObjectID, units []repofile.Unit) ([]byte, error) {
	locations := make([]repofile.ObjectUnitLocation, 0, len(units))
	for _, unit := range units {
		ref := unit.SchemaRef
		locations = append(locations, repofile.ObjectUnitLocation{Address: unit.Address, Path: unit.Path, SchemaRef: &ref})
	}
	return repofile.EncodeObjectUnitLocations(objectID, locations)
}

func treeChangeSet(cs objectmodel.ChangeSet, changes []snapshot.TreeChange) snapshot.TreeChangeSet {
	return snapshot.TreeChangeSet{
		TargetRepository:     cs.TargetRepository,
		TargetRef:            cs.TargetRef,
		BaseCommit:           cs.BaseCommit,
		ExpectedTargetCommit: cs.ExpectedTargetCommit,
		Changes:              changes,
		Message:              cs.Message,
		Author:               cs.Author,
		RequestID:            cs.RequestID,
		RuleID:               cs.RuleID,
	}
}

func requireIncrementalLocatorLayout(tree snapshot.TreeStore, commit kernel.CommitID) error {
	if raw, err := tree.ReadFile(repofile.LocatorCompletePath, commit); err == nil {
		if string(raw) == repofile.LocatorCompleteBody {
			referrers, err := tree.ReadFile(repofile.LocatorReferrersCompletePath, commit)
			if err == nil && string(referrers) == repofile.LocatorReferrersCompleteBody {
				return nil
			}
			if err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
				return err
			}
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"object locator lacks complete Schema referrers; explicitly rebuild locators before writing")
		}
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"outdated object locator layout requires explicit migration before writing")
	} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return err
	}
	if _, err := tree.ReadFile(repofile.LocatorManifestPath, commit); err == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"legacy whole-repository object locator requires explicit migration before writing")
	} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return err
	}
	return nil
}

func changedObjectIDs(operations []objectmodel.Operation) []objectmodel.ObjectID {
	seen := map[objectmodel.ObjectID]struct{}{}
	ids := make([]objectmodel.ObjectID, 0, len(operations))
	for _, op := range operations {
		if _, ok := seen[op.Address.ObjectID]; ok {
			continue
		}
		seen[op.Address.ObjectID] = struct{}{}
		ids = append(ids, op.Address.ObjectID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func readChangedObjects(tree snapshot.TreeStore, commit kernel.CommitID, operations []objectmodel.Operation) (*repofile.Tree, error) {
	ids := changedObjectIDs(operations)
	prefetchChangedObjects(tree, commit, ids)
	idx := repofile.NewTree()
	for _, objectID := range ids {
		units, err := readObject(tree, objectID, commit)
		if err != nil {
			return nil, err
		}
		for _, unit := range units {
			unitCopy := unit
			if err := repofile.Ingest(idx, &unitCopy, unit.Path); err != nil {
				return nil, err
			}
		}
	}
	return idx, nil
}

// prefetchChangedObjects warms the planning view in two bounded fan-outs:
// first every changed object's locator, then every unit path those locators
// name. The serial read in readChangedObjects then consumes cached
// coordinates in order and reports failures exactly as before.
func prefetchChangedObjects(tree snapshot.TreeReader, commit kernel.CommitID, ids []objectmodel.ObjectID) {
	if _, ok := tree.(basisPrefetcher); !ok || len(ids) < 2 {
		return
	}
	locators := make([]string, 0, len(ids))
	for _, id := range ids {
		locators = append(locators, repofile.ObjectLocatorPath(id))
	}
	prefetchBasis(tree, locators, commit)
	units := []string{}
	for _, id := range ids {
		paths, err := repofile.ReadObjectLocator(tree, id, commit)
		if err != nil {
			return
		}
		units = append(units, paths...)
	}
	prefetchBasis(tree, units, commit)
}

func readObject(tree snapshot.TreeReader, objectID objectmodel.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
	paths, err := repofile.ReadObjectLocator(tree, objectID, commit)
	if err != nil {
		return nil, err
	}
	idx := repofile.NewTree()
	for _, unitPath := range paths {
		content, err := tree.ReadFile(unitPath, commit)
		if err != nil {
			return nil, err
		}
		unit := repofile.Parse(string(content))
		if unit == nil || unit.ObjectID != objectID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"object locator returned a mismatched unit for %s", objectID)
		}
		if err := repofile.Ingest(idx, unit, unitPath); err != nil {
			return nil, err
		}
	}
	return idx.ObjectUnits(objectID), nil
}
