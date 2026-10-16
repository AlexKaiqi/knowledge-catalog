package writer

import (
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// validateSchemaRefs validates every side of the Schema contract before a
// Snapshot mutation:
//
//   - every schema/* PUT conforms to the built-in System Meta Schema;
//   - reusing a schema object ID stays compatible AND keeps the instances that
//     already reference it valid at the target basis;
//   - removing a schema/* object requires proof that nothing still references it;
//   - every non-binding PUT conforms to its *effective* Domain Schema, whether
//     the ChangeSet states schema_ref explicitly or inherits it from the unit
//     already stored at that Address.
//
// A normal schema_ref never resolves through another Repository. The System
// Meta Schema is a protocol trust root, not a cross-Repository instance ref.
func validateSchemaRefs(registry snapshot.Registry, target snapshot.Store, cs objectmodel.ChangeSet) error {
	drafts := map[objectmodel.ObjectID]objectmodel.SchemaDefinition{}
	removedSchemas := map[objectmodel.ObjectID]struct{}{}
	instancePuts := false
	for _, op := range cs.Operations {
		schemaObject := objectmodel.IsSchemaObject(op.Address.ObjectID)
		switch {
		case op.Op == objectmodel.OpPut && schemaObject:
			definition, err := objectmodel.ParseSchemaDefinition(op.Address.ObjectID, op.Value)
			if err != nil {
				return err
			}
			if err := objectmodel.AssertProtocolSchemaPublication(op.Address.ObjectID, op.Value); err != nil {
				return err
			}
			drafts[op.Address.ObjectID] = definition
		case op.Op == objectmodel.OpRemove && schemaObject:
			removedSchemas[op.Address.ObjectID] = struct{}{}
		case op.Op == objectmodel.OpPut:
			instancePuts = true
		}
	}
	if !instancePuts && len(drafts) == 0 && len(removedSchemas) == 0 {
		return nil
	}

	resolver, err := newSchemaResolver(target, cs)
	if err != nil {
		return err
	}
	if err := resolver.prepareFinalUnits(cs); err != nil {
		return err
	}
	if err := resolver.validateIndexDeclarations(registry, cs, drafts); err != nil {
		return err
	}
	if err := resolver.validateValueSources(registry, cs); err != nil {
		return err
	}

	removedAddresses, batchValues := resolver.batchEffects(cs)
	if err := resolver.finalSchemaChanges(drafts, removedSchemas); err != nil {
		return err
	}
	for _, objectID := range sortedSchemaIDs(drafts) {
		if err := resolver.checkSchemaUpdate(objectID, drafts[objectID], removedAddresses, batchValues); err != nil {
			return err
		}
	}
	for _, objectID := range sortedSchemaIDs(removedSchemas) {
		if err := resolver.checkSchemaRemoval(objectID, removedAddresses); err != nil {
			return err
		}
	}

	for _, op := range sortedBatchOperations(batchValues) {
		if op.Op != objectmodel.OpPut || objectmodel.IsSchemaObject(op.Address.ObjectID) {
			continue
		}
		ref, err := resolver.effectiveSchemaRef(op)
		if err != nil {
			return err
		}
		schemaObjectID := objectmodel.ObjectID("")
		if ref != "" {
			if parsed, ok := objectmodel.ParseSchemaRef(ref); ok {
				schemaObjectID = parsed.Object
			}
		}
		if err := objectmodel.AssertReadmeBinding(op.Address, schemaObjectID); err != nil {
			return err
		}
		if ref == "" {
			continue
		}
		definition, err := resolver.definitionFor(ref, drafts)
		if err != nil {
			return err
		}
		if r := resolver.final[objectmodel.AddressKey(op.Address)]; r.ValueSource.IsBinding() {
			// A binding unit has no Snapshot body; only its Address shape is
			// governed by the structure-only Schema.
			if err := objectmodel.ValidateSchemaAddress(op.Address, definition); err != nil {
				return err
			}
			continue
		}
		if err := objectmodel.ValidateSchemaInstance(op.Address, op.Value, definition); err != nil {
			return err
		}
	}
	return nil
}

func sortedSchemaIDs[V any](values map[objectmodel.ObjectID]V) []objectmodel.ObjectID {
	ids := make([]objectmodel.ObjectID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// schemaResolver reads the fixed target basis. Native providers answer through
// bounded point lookups and their reverse schema index. Tree providers read only
// affected objects, indexed referrers and selected Schema bodies.
type schemaResolver struct {
	target      snapshot.Store
	native      objectmodel.NativeRepository
	nativeOK    bool
	tree        snapshot.TreeStore
	treeOK      bool
	at          kernel.CommitID
	before      map[string]objectmodel.Unit
	final       map[string]objectmodel.Unit
	definitions map[string]objectmodel.SchemaDefinition
}

func newSchemaResolver(target snapshot.Store, cs objectmodel.ChangeSet) (*schemaResolver, error) {
	native, nativeOK := target.(objectmodel.NativeRepository)
	tree, treeOK := snapshot.TreeStoreOf(target)
	if !nativeOK && !treeOK {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"repository %s has no immutable object access for schema resolution", target.ID())
	}
	at := cs.ExpectedTargetCommit
	if at == "" {
		head, err := target.Head(cs.TargetRef)
		if err != nil {
			return nil, err
		}
		at = head
	}
	return &schemaResolver{
		target: target, native: native, nativeOK: nativeOK, tree: tree, treeOK: treeOK,
		at: at, definitions: map[string]objectmodel.SchemaDefinition{},
	}, nil
}

func (r *schemaResolver) schemaValue(objectID objectmodel.ObjectID, commit kernel.CommitID) (any, error) {
	if r.nativeOK {
		value, err := r.native.Read(objectID, commit)
		if err != nil {
			return nil, err
		}
		return value.Value, nil
	}
	value, found, err := storeObjectValue(r.target, objectID, commit)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema %s is missing at commit %s", objectID, commit)
	}
	return value, nil
}

// effectiveSchemaRef is the schema_ref this PUT will actually be stored with.
// An operation that omits schema_ref inherits the declaration already on that
// Address, so it must be held to the same contract instead of skipping
// validation entirely.
func (r *schemaResolver) effectiveSchemaRef(op objectmodel.Operation) (string, error) {
	return strings.TrimSpace(r.final[objectmodel.AddressKey(op.Address)].SchemaRef), nil
}

// definitionFor resolves one schema_ref against this ChangeSet's drafts or the
// fixed target basis. A ref naming another Repository is always rejected.
func (r *schemaResolver) definitionFor(ref string, drafts map[objectmodel.ObjectID]objectmodel.SchemaDefinition) (objectmodel.SchemaDefinition, error) {
	if cached, ok := r.definitions[ref]; ok {
		return cached, nil
	}
	parsed, parsedOK := objectmodel.ParseSchemaRef(ref)
	if !parsedOK {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema_ref %q is not a schema/* object", ref)
	}
	if parsed.Repository == "" && parsed.Commit == "" {
		if trusted, ok := objectmodel.ProtocolTrustRootSchema(parsed.Object); ok {
			r.definitions[ref] = trusted
			return trusted, nil
		}
	}
	if parsed.Repository != "" && parsed.Repository != r.target.ID() {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema_ref %q must name the target repository", ref)
	}
	if parsed.Commit == "" {
		if draft, exists := drafts[parsed.Object]; exists {
			r.definitions[ref] = draft
			return draft, nil
		}
	}
	commit := r.at
	if parsed.Commit != "" {
		if !r.target.HasCommit(parsed.Commit) {
			return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
				"schema_ref %q commit does not exist", ref)
		}
		commit = parsed.Commit
	}
	value, err := r.schemaValue(parsed.Object, commit)
	if err != nil {
		if code := kernel.CodeOf(err); code != kernel.ErrObjectRefUnresolved && code != kernel.ErrSchemaRevisionUnresolved {
			return objectmodel.SchemaDefinition{}, err
		}
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema_ref %q does not resolve to a schema object", ref)
	}
	definition, err := objectmodel.ParseSchemaDefinition(parsed.Object, value)
	if err != nil {
		return objectmodel.SchemaDefinition{}, err
	}
	r.definitions[ref] = definition
	return definition, nil
}

// checkSchemaUpdate enforces that republishing a schema object ID is both
// contract-compatible and safe for the instances already published against it.
func (r *schemaResolver) checkSchemaUpdate(
	objectID objectmodel.ObjectID,
	draft objectmodel.SchemaDefinition,
	removedAddresses map[string]struct{},
	batchValues map[string]objectmodel.Operation,
) error {
	current, readErr := r.schemaValue(objectID, r.at)
	if readErr != nil {
		if code := kernel.CodeOf(readErr); code == kernel.ErrObjectRefUnresolved || code == kernel.ErrSchemaRevisionUnresolved {
			// First publication: nothing can reference it yet.
			return nil
		}
		return readErr
	}
	previous, parseErr := objectmodel.ParseSchemaDefinition(objectID, current)
	if parseErr != nil {
		return parseErr
	}
	if breaking := objectmodel.BreakingSchemaChanges(previous, draft); len(breaking) > 0 {
		return kernel.Fail(kernel.ErrSchemaIncompatible,
			"schema %s has breaking changes (%s); publish a new major schema object_id", objectID, strings.Join(breaking, "; "))
	}
	referrers, err := r.referrers(objectID)
	if err != nil {
		return err
	}
	for _, address := range referrers {
		key := objectmodel.AddressKey(address)
		if _, removed := removedAddresses[key]; removed {
			continue
		}
		if _, rewritten := batchValues[key]; rewritten {
			// This ChangeSet replaces the value; the instance pass validates it
			// against the same draft.
			continue
		}
		value, unit, err := r.unitValue(address)
		if err != nil {
			return err
		}
		if !unit {
			continue
		}
		if err := objectmodel.ValidateSchemaInstance(address, value, draft); err != nil {
			return kernel.Fail(kernel.ErrSchemaInstanceInvalid,
				"schema %s cannot be updated in place: %v", objectID, err)
		}
	}
	return nil
}

// checkSchemaRemoval refuses to delete a Domain Schema that instances still
// reference at the target basis.
func (r *schemaResolver) checkSchemaRemoval(objectID objectmodel.ObjectID, removedAddresses map[string]struct{}) error {
	referrers, err := r.referrers(objectID)
	if err != nil {
		return err
	}
	remainingSet := map[string]struct{}{}
	for _, address := range referrers {
		if _, removed := removedAddresses[objectmodel.AddressKey(address)]; removed {
			continue
		}
		if r.final != nil {
			if unit, changed := r.final[objectmodel.AddressKey(address)]; changed {
				parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
				if !ok || parsed.Object != objectID {
					continue
				}
			}
		}
		remainingSet[objectmodel.AddressKey(address)] = struct{}{}
	}
	for _, unit := range r.final {
		if objectmodel.IsSchemaObject(unit.ObjectID) {
			continue
		}
		if parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef); ok && parsed.Object == objectID {
			remainingSet[objectmodel.AddressKey(unit.Address)] = struct{}{}
		}
	}
	remaining := make([]string, 0, len(remainingSet))
	for address := range remainingSet {
		remaining = append(remaining, address)
	}
	if len(remaining) > 0 {
		sort.Strings(remaining)
		shown := remaining
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return kernel.Fail(kernel.ErrSchemaIncompatible,
			"schema %s still has %d referencing address(es) (%s); remove or migrate them first",
			objectID, len(remaining), strings.Join(shown, ", "))
	}
	return nil
}

// referrers is the bounded reverse schema_ref lookup. Native providers must
// answer from their own index; tree providers use fixed derived coordinates.
func (r *schemaResolver) referrers(schema objectmodel.ObjectID) ([]objectmodel.Address, error) {
	if r.nativeOK {
		locator, ok := r.native.(objectmodel.SchemaReferrerLocator)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s cannot prove which instances reference schema %s", r.target.ID(), schema)
		}
		return locator.SchemaReferrerAddresses(schema, r.at)
	}
	return repofile.ReadSchemaReferrers(r.tree, schema, r.at)
}

// unitValue reads one already-published unit at the target basis. Binding units
// have no inline Snapshot value and report false.
func (r *schemaResolver) unitValue(address objectmodel.Address) (any, bool, error) {
	if r.nativeOK {
		resolution, err := r.native.ResolveAddress(address, r.at)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				return nil, false, nil
			}
			return nil, false, err
		}
		if resolution.Status != objectmodel.StatusResolved {
			return nil, false, nil
		}
		if resolution.ValueSource != nil && resolution.ValueSource.Kind == objectmodel.ValueSourceBinding {
			return nil, false, nil
		}
		value, err := r.native.ReadAddress(address, r.at)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				return nil, false, nil
			}
			return nil, false, err
		}
		return value.Value, true, nil
	}
	unit, ok, err := repofile.ReadAddressUnit(r.tree, address, r.at)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if unit.ValueSource != nil && unit.ValueSource.Kind == objectmodel.ValueSourceBinding {
		return nil, false, nil
	}
	return unit.Value, true, nil
}
