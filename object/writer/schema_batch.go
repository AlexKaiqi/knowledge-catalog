package writer

import (
	objectmodel "kc/object"
	repofile "kc/object/format"
	"sort"

	"kc/kernel"
)

// The file-backed maintenance plan uses the same final unit algebra as commit.
// It prevents deleted or migrated units from retaining obligations to old refs.
func (r *schemaResolver) prepareFinalUnits(cs objectmodel.ChangeSet) error {
	if r.nativeOK {
		return r.prepareNativeFinalUnits(cs)
	}
	idx, err := readChangedObjects(r.tree, r.at, cs.Operations)
	if err != nil {
		return err
	}
	r.before = schemaPlanningUnits(idx)
	if err := repofile.ApplyOperations(idx, cs.Operations, cs.Provenance, map[string]string{}, map[string]struct{}{}); err != nil {
		return err
	}
	r.final = schemaPlanningUnits(idx)
	r.finalByID = nil
	return nil
}

func schemaPlanningUnits(idx *repofile.Tree) map[string]objectmodel.Unit {
	files := make([]repofile.Unit, 0, len(idx.Units))
	for _, unit := range idx.Units {
		files = append(files, unit)
	}
	units := make(map[string]objectmodel.Unit, len(files))
	for _, unit := range repofile.ToObjectUnits(files) {
		units[objectmodel.AddressKey(unit.Address)] = unit
	}
	return units
}

func (r *schemaResolver) batchEffects(cs objectmodel.ChangeSet) (map[string]struct{}, map[string]objectmodel.Operation) {
	removed := map[string]struct{}{}
	for key := range r.before {
		if _, present := r.final[key]; !present {
			removed[key] = struct{}{}
		}
	}
	written := map[string]objectmodel.Operation{}
	for _, operation := range cs.Operations {
		if operation.Op != objectmodel.OpPut || objectmodel.IsSchemaObject(operation.Address.ObjectID) {
			continue
		}
		key := objectmodel.AddressKey(operation.Address)
		unit, present := r.final[key]
		if !present {
			continue
		}
		written[key] = objectmodel.Operation{Op: objectmodel.OpPut, Address: unit.Address, SchemaRef: unit.SchemaRef, Value: unit.Value}
	}
	return removed, written
}

func (r *schemaResolver) finalSchemaChanges(drafts map[objectmodel.ObjectID]objectmodel.SchemaDefinition, removed map[objectmodel.ObjectID]struct{}) error {
	for id := range drafts {
		units := r.finalObjectUnits(id)
		if len(units) == 0 {
			delete(drafts, id)
			continue
		}
		value, err := objectmodel.AssembleUnits(units)
		if err != nil {
			return err
		}
		definition, err := objectmodel.ParseSchemaDefinition(id, value)
		if err != nil {
			return err
		}
		drafts[id] = definition
	}
	for id := range removed {
		if len(r.finalObjectUnits(id)) != 0 {
			delete(removed, id)
		}
	}
	return nil
}

// finalObjectUnits answers per-object lookups from an index built once over
// the final plan; the plan is fixed after prepareFinalUnits, which resets it.
func (r *schemaResolver) finalObjectUnits(id objectmodel.ObjectID) []objectmodel.Unit {
	if r.finalByID == nil {
		r.finalByID = make(map[objectmodel.ObjectID][]objectmodel.Unit, len(r.final))
		for _, unit := range r.final {
			r.finalByID[unit.ObjectID] = append(r.finalByID[unit.ObjectID], unit)
		}
	}
	return r.finalByID[id]
}

func (r *schemaResolver) prepareNativeFinalUnits(cs objectmodel.ChangeSet) error {
	existing := []objectmodel.Unit{}
	for _, id := range changedObjectIDs(cs.Operations) {
		value, err := r.native.Read(id, r.at)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		}
		if err != nil {
			return err
		}
		units, err := r.nativePlanningUnits(id, value)
		if err != nil {
			return err
		}
		existing = append(existing, units...)
	}
	final, _, err := objectmodel.ApplyUnits(existing, cs.Operations, cs.Provenance)
	if err != nil {
		return err
	}
	r.before, r.final, r.finalByID = map[string]objectmodel.Unit{}, map[string]objectmodel.Unit{}, nil
	for _, unit := range existing {
		r.before[objectmodel.AddressKey(unit.Address)] = unit
	}
	for _, unit := range final {
		r.final[objectmodel.AddressKey(unit.Address)] = unit
	}
	return nil
}

func (r *schemaResolver) nativePlanningUnits(id objectmodel.ObjectID, value objectmodel.ObjectValue) ([]objectmodel.Unit, error) {
	addresses := map[string]objectmodel.Address{}
	for _, declaration := range value.Declarations {
		addresses[objectmodel.AddressKey(declaration.Address)] = declaration.Address
	}
	if len(addresses) == 0 {
		for _, address := range value.Units {
			addresses[objectmodel.AddressKey(address)] = address
		}
	}
	if len(addresses) == 0 {
		address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}
		addresses[objectmodel.AddressKey(address)] = address
	}
	units := make([]objectmodel.Unit, 0, len(addresses))
	for _, address := range addresses {
		resolution, err := r.native.ResolveAddress(address, r.at)
		if err != nil {
			return nil, err
		}
		if resolution.Status != objectmodel.StatusResolved || resolution.Address.ObjectID != id {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "native repository %s lacks maintenance coordinates for %s", r.target.ID(), id)
		}
		body, err := r.native.ReadAddress(address, r.at)
		if err != nil {
			return nil, err
		}
		units = append(units, objectmodel.Unit{ObjectID: id, Address: resolution.Address,
			SchemaRef: resolution.SchemaRef, ValueSource: resolution.ValueSource,
			Value: body.Value, Digest: resolution.Digest})
	}
	return units, nil
}

func sortedBatchOperations(operations map[string]objectmodel.Operation) []objectmodel.Operation {
	keys := make([]string, 0, len(operations))
	for key := range operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]objectmodel.Operation, 0, len(keys))
	for _, key := range keys {
		result = append(result, operations[key])
	}
	return result
}
