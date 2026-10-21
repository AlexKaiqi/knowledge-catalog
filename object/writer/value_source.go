package writer

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// validateBindingOperation holds one PUT to the LiveAccess declaration shape:
// a binding unit declares where its value comes from and carries no Snapshot
// body; Schema objects and relations never declare a value source; and a body
// cannot smuggle a second declaration inside itself.
func validateBindingOperation(op objectmodel.Operation) error {
	if body, ok := op.Value.(map[string]any); ok {
		if _, nested := body["valueSource"]; nested {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"%s body contains valueSource; declare it on the unit, not inside the value", objectmodel.AddressLabel(op.Address))
		}
	}
	if !op.ValueSource.IsBinding() {
		return nil
	}
	if objectmodel.IsSchemaObject(op.Address.ObjectID) {
		return kernel.Fail(kernel.ErrUsageInvalid,
			"schema %s describes structure only and cannot declare a valueSource", op.Address.ObjectID)
	}
	if op.Address.Kind != objectmodel.KindAspect && op.Address.Kind != objectmodel.KindMember {
		return kernel.Fail(kernel.ErrUsageInvalid,
			"valueSource is declared on an Aspect or Member, not on %s %s", op.Address.Kind, objectmodel.AddressLabel(op.Address))
	}
	if op.Value != nil {
		return kernel.Fail(kernel.ErrUsageInvalid,
			"%s declares a binding valueSource and must not carry a Snapshot value", objectmodel.AddressLabel(op.Address))
	}
	return nil
}

// validateValueSources checks the final units this ChangeSet produces:
// every published ResourceDescriptor parses, every binding resolves its
// descriptorRef to exactly one ResourceDescriptor. Aspect-level and
// Member-level declarations of one Aspect are exclusive because an Aspect is
// stored either as one Record unit or as Member units, never both.
//
// descriptorRef resolves in a fixed order: this ChangeSet's final units, the
// target basis, then the DefaultRef head of every other Repository. A
// descriptor held by more than one other Repository is ambiguous.
func (r *schemaResolver) validateValueSources(registry snapshot.Registry, cs objectmodel.ChangeSet) error {
	changed := map[objectmodel.ObjectID]struct{}{}
	for _, op := range cs.Operations {
		changed[op.Address.ObjectID] = struct{}{}
	}
	for _, id := range sortedSchemaIDs(changed) {
		if objectmodel.IsSchemaObject(id) {
			continue
		}
		units := r.finalObjectUnits(id)
		if len(units) == 0 {
			continue
		}
		value, err := objectmodel.AssembleUnits(units)
		if err != nil {
			return err
		}
		if objectmodel.IsResourceDescriptorValue(value) {
			if _, err := objectmodel.ParseResourceDescriptor(id, value); err != nil {
				return err
			}
		}
	}
	resolved := map[objectmodel.ObjectID]struct{}{}
	for _, key := range sortedFinalKeys(r.final) {
		unit := r.final[key]
		if _, touched := changed[unit.ObjectID]; !touched || !unit.ValueSource.IsBinding() {
			continue
		}
		ref := unit.ValueSource.Binding.DescriptorRef
		if _, done := resolved[ref]; done {
			continue
		}
		if err := r.resolveDescriptorRef(registry, changed, ref); err != nil {
			return err
		}
		resolved[ref] = struct{}{}
	}
	return nil
}

func (r *schemaResolver) resolveDescriptorRef(registry snapshot.Registry, changed map[objectmodel.ObjectID]struct{}, ref objectmodel.ObjectID) error {
	if _, inBatch := changed[ref]; inBatch {
		units := r.finalObjectUnits(ref)
		if len(units) == 0 {
			return kernel.Fail(kernel.ErrObjectRefUnresolved, "descriptorRef %s is removed by this changeset", ref)
		}
		value, err := objectmodel.AssembleUnits(units)
		if err != nil {
			return err
		}
		return requireDescriptor(ref, value, r.target.ID())
	}
	value, found, err := storeObjectValue(r.target, ref, r.at)
	if err != nil {
		return err
	}
	if found {
		return requireDescriptor(ref, value, r.target.ID())
	}
	holders := []kernel.RepositoryID{}
	var held any
	ids := registry.IDs()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, repoID := range ids {
		if repoID == r.target.ID() {
			continue
		}
		store, ok := registry.Get(repoID)
		if !ok {
			continue
		}
		head, err := store.Head(snapshot.DefaultRef)
		if err != nil {
			return err
		}
		if head == "" {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"repository %s has no published head while resolving descriptorRef %s", repoID, ref)
		}
		value, found, err := storeObjectValue(store, ref, head)
		if err != nil {
			return err
		}
		if found {
			holders = append(holders, repoID)
			held = value
		}
	}
	switch len(holders) {
	case 0:
		return kernel.Fail(kernel.ErrObjectRefUnresolved, "descriptorRef %s does not resolve to a ResourceDescriptor", ref)
	case 1:
		return requireDescriptor(ref, held, holders[0])
	default:
		return kernel.Fail(kernel.ErrObjectIDConflict,
			"descriptorRef %s is held by several repositories (%v); descriptor IDs must be unique", ref, holders)
	}
}

func requireDescriptor(ref objectmodel.ObjectID, value any, repo kernel.RepositoryID) error {
	if !objectmodel.IsResourceDescriptorValue(value) {
		return kernel.Fail(kernel.ErrUsageInvalid, "descriptorRef %s in %s is not a ResourceDescriptor", ref, repo)
	}
	_, err := objectmodel.ParseResourceDescriptor(ref, value)
	return err
}

// storeObjectValue reads one assembled object at a fixed commit through the
// provider's immutable object access; a missing object reports false.
func storeObjectValue(store snapshot.Store, id objectmodel.ObjectID, commit kernel.CommitID) (any, bool, error) {
	if native, ok := store.(objectmodel.NativeRepository); ok {
		value, err := native.Read(id, commit)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		return value.Value, true, nil
	}
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s has no immutable object access", store.ID())
	}
	units, err := readObject(tree, id, commit)
	if err != nil {
		return nil, false, err
	}
	if len(units) == 0 {
		return nil, false, nil
	}
	value, err := repofile.Assemble(units)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func sortedFinalKeys(units map[string]objectmodel.Unit) []string {
	keys := make([]string, 0, len(units))
	for key := range units {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
