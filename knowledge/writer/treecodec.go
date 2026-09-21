package writer

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// applyKnowledgeCommit is the sole ② → ⓪ codec. Snapshot adapters receive
// only literal path changes and never see Address, Aspect, or PUT/REMOVE.
func applyKnowledgeCommit(commandID string, target snapshot.Store, cs knowledge.ChangeSet) (kernel.CommitID, error) {
	if native, ok := target.(knowledge.ChangeStore); ok {
		// An omitted path retains the existing unit's location. Only the native
		// provider knows whether this is a new unit needing a default path.
		return native.ApplyKnowledgeChange(commandID, cs)
	}
	if _, native := target.(knowledge.NativeRepository); native {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"native repository %s does not provide typed knowledge writes", target.ID())
	}
	tree, ok := snapshot.TreeStoreOf(target)
	if !ok {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s is mounted as a plain snapshot and cannot accept knowledge changes without tree access", target.ID())
	}
	if err := requireIncrementalLocatorLayout(tree, cs.BaseCommit); err != nil {
		return "", err
	}

	idx, err := readChangedKnowledgeObjects(tree, cs.BaseCommit, cs.Operations)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(idx, op, cs.Provenance, toWrite, toDelete); err != nil {
			return "", err
		}
	}
	for _, objectID := range changedObjectIDs(cs.Operations) {
		units := idx.ObjectUnits(objectID)
		locatorPath := repofile.ObjectLocatorPath(objectID)
		if len(units) == 0 {
			toDelete[locatorPath] = struct{}{}
			continue
		}
		paths := make([]string, 0, len(units))
		for _, unit := range units {
			paths = append(paths, unit.Path)
		}
		locator, err := repofile.EncodeObjectLocator(objectID, paths)
		if err != nil {
			return "", err
		}
		toWrite[locatorPath] = string(locator)
	}
	if err := updateSchemaLocator(tree, cs.BaseCommit, idx, cs.Operations, toWrite); err != nil {
		return "", err
	}
	toWrite[repofile.LocatorCompletePath] = repofile.LocatorCompleteBody

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

	return tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     cs.TargetRepository,
		TargetRef:            cs.TargetRef,
		BaseCommit:           cs.BaseCommit,
		ExpectedTargetCommit: cs.ExpectedTargetCommit,
		Changes:              changes,
		Message:              cs.Message,
		Author:               cs.Author,
		RequestID:            cs.RequestID,
		RuleID:               cs.RuleID,
	})
}

func requireIncrementalLocatorLayout(tree snapshot.TreeStore, commit kernel.CommitID) error {
	if raw, err := tree.ReadFile(repofile.LocatorCompletePath, commit); err == nil {
		if string(raw) == repofile.LocatorCompleteBody {
			return nil
		}
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"outdated knowledge locator layout requires explicit migration before writing")
	} else if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		return err
	}
	if _, err := tree.ReadFile(repofile.LocatorManifestPath, commit); err == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"legacy whole-repository knowledge locator requires explicit migration before writing")
	} else if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		return err
	}
	return nil
}

func updateSchemaLocator(tree snapshot.TreeStore, commit kernel.CommitID, idx *repofile.Tree, operations []knowledge.Operation, toWrite map[string]string) error {
	changed := false
	for _, op := range operations {
		if knowledge.IsSchemaObject(op.Address.ObjectID) {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}
	ids := []knowledge.ObjectID{}
	raw, err := tree.ReadFile(repofile.LocatorSchemaIndexPath, commit)
	if err == nil {
		ids, err = repofile.DecodeSchemaIndex(raw)
		if err != nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "invalid schema locator at %s: %v", commit, err)
		}
	} else if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		return err
	}
	present := map[knowledge.ObjectID]struct{}{}
	for _, id := range ids {
		present[id] = struct{}{}
	}
	for _, op := range operations {
		if !knowledge.IsSchemaObject(op.Address.ObjectID) {
			continue
		}
		if len(idx.ObjectUnits(op.Address.ObjectID)) == 0 {
			delete(present, op.Address.ObjectID)
		} else {
			present[op.Address.ObjectID] = struct{}{}
		}
	}
	ids = ids[:0]
	for id := range present {
		ids = append(ids, id)
	}
	encoded, err := repofile.EncodeSchemaIndex(ids)
	if err != nil {
		return err
	}
	toWrite[repofile.LocatorSchemaIndexPath] = string(encoded)
	return nil
}

func changedObjectIDs(operations []knowledge.Operation) []knowledge.ObjectID {
	seen := map[knowledge.ObjectID]struct{}{}
	ids := make([]knowledge.ObjectID, 0, len(operations))
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

func readChangedKnowledgeObjects(tree snapshot.TreeStore, commit kernel.CommitID, operations []knowledge.Operation) (*repofile.Tree, error) {
	idx := repofile.NewTree()
	for _, objectID := range changedObjectIDs(operations) {
		units, err := readKnowledgeObject(tree, objectID, commit)
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

func readKnowledgeObject(tree snapshot.TreeStore, objectID knowledge.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
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

func readKnowledgeTree(tree snapshot.TreeStore, commit kernel.CommitID) (*repofile.Tree, error) {
	paths, err := tree.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	idx := repofile.NewTree()
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		content, err := tree.ReadFile(path, commit)
		if err != nil {
			return nil, err
		}
		unit := repofile.Parse(string(content))
		if unit == nil {
			continue
		}
		if err := repofile.Ingest(idx, unit, path); err != nil {
			return nil, err
		}
	}
	return idx, nil
}
