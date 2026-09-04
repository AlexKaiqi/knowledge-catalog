package writer

import (
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
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
func validateSchemaRefs(target snapshot.Store, cs knowledge.ChangeSet) error {
	drafts := map[knowledge.ObjectID]knowledge.SchemaDefinition{}
	removedSchemas := map[knowledge.ObjectID]struct{}{}
	instancePuts := false
	for _, op := range cs.Operations {
		schemaObject := knowledge.IsSchemaObject(op.Address.ObjectID)
		switch {
		case op.Op == knowledge.OpPut && schemaObject:
			definition, err := knowledge.ParseSchemaDefinition(op.Address.ObjectID, op.Value)
			if err != nil {
				return err
			}
			if err := knowledge.AssertProtocolSchemaPublication(op.Address.ObjectID, op.Value); err != nil {
				return err
			}
			drafts[op.Address.ObjectID] = definition
		case op.Op == knowledge.OpRemove && schemaObject:
			removedSchemas[op.Address.ObjectID] = struct{}{}
		case op.Op == knowledge.OpPut:
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

	// Addresses removed by this ChangeSet no longer constrain a Schema change.
	removedAddresses := map[string]struct{}{}
	for _, op := range cs.Operations {
		if op.Op == knowledge.OpRemove {
			removedAddresses[knowledge.AddressKey(op.Address)] = struct{}{}
		}
	}
	// Values written by this ChangeSet are validated from the batch, not from
	// the pre-existing basis.
	batchValues := map[string]knowledge.Operation{}
	for _, op := range cs.Operations {
		if op.Op == knowledge.OpPut && !knowledge.IsSchemaObject(op.Address.ObjectID) {
			batchValues[knowledge.AddressKey(op.Address)] = op
		}
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

	for _, op := range cs.Operations {
		if op.Op != knowledge.OpPut || knowledge.IsSchemaObject(op.Address.ObjectID) {
			continue
		}
		ref, err := resolver.effectiveSchemaRef(op)
		if err != nil {
			return err
		}
		schemaObjectID := knowledge.ObjectID("")
		if ref != "" {
			if parsed, ok := knowledge.ParseSchemaRef(ref); ok {
				schemaObjectID = parsed.Object
			}
		}
		if err := knowledge.AssertSourceProfileBinding(op.Address, schemaObjectID); err != nil {
			return err
		}
		if ref == "" {
			continue
		}
		definition, err := resolver.definitionFor(ref, drafts)
		if err != nil {
			return err
		}
		// A Binding declaration carries no inline Snapshot value; its logical
		// value is validated when the serving layer hydrates it.
		if op.ValueSource != nil && op.ValueSource.Kind == knowledge.ValueSourceBinding {
			continue
		}
		if err := knowledge.ValidateSchemaInstance(op.Address, op.Value, definition); err != nil {
			return err
		}
	}
	return nil
}

func sortedSchemaIDs[V any](values map[knowledge.ObjectID]V) []knowledge.ObjectID {
	ids := make([]knowledge.ObjectID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// schemaResolver reads the fixed target basis. Native providers answer through
// bounded point lookups and their reverse schema index; tree providers reuse the
// single decoded tree the codec needs for this commit anyway.
type schemaResolver struct {
	target      snapshot.Store
	native      knowledge.NativeRepository
	nativeOK    bool
	tree        snapshot.TreeStore
	treeOK      bool
	at          kernel.CommitID
	index       *repofile.Tree
	definitions map[string]knowledge.SchemaDefinition
}

func newSchemaResolver(target snapshot.Store, cs knowledge.ChangeSet) (*schemaResolver, error) {
	native, nativeOK := target.(knowledge.NativeRepository)
	tree, treeOK := snapshot.TreeStoreOf(target)
	if !nativeOK && !treeOK {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"repository %s has no immutable knowledge access for schema resolution", target.ID())
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
		at: at, definitions: map[string]knowledge.SchemaDefinition{},
	}, nil
}

func (r *schemaResolver) treeIndex() (*repofile.Tree, error) {
	if r.index != nil {
		return r.index, nil
	}
	index, err := readKnowledgeTree(r.tree, r.at)
	if err != nil {
		return nil, err
	}
	r.index = index
	return index, nil
}

func (r *schemaResolver) schemaValue(objectID knowledge.ObjectID, commit kernel.CommitID) (any, error) {
	if r.nativeOK {
		value, err := r.native.Read(objectID, commit)
		if err != nil {
			return nil, err
		}
		return value.Value, nil
	}
	if !r.treeOK {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"repository %s cannot resolve schema %s", r.target.ID(), objectID)
	}
	index := r.index
	if commit != r.at || index == nil {
		var err error
		if commit == r.at {
			index, err = r.treeIndex()
		} else {
			index, err = readKnowledgeTree(r.tree, commit)
		}
		if err != nil {
			return nil, err
		}
	}
	units := index.ObjectUnits(objectID)
	if len(units) == 0 {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema %s is missing at commit %s", objectID, commit)
	}
	return repofile.Assemble(units)
}

// effectiveSchemaRef is the schema_ref this PUT will actually be stored with.
// An operation that omits schema_ref inherits the declaration already on that
// Address, so it must be held to the same contract instead of skipping
// validation entirely.
func (r *schemaResolver) effectiveSchemaRef(op knowledge.Operation) (string, error) {
	if explicit := strings.TrimSpace(op.SchemaRef); explicit != "" {
		return explicit, nil
	}
	if r.nativeOK {
		resolution, err := r.native.ResolveAddress(op.Address, r.at)
		if err != nil {
			if code := kernel.CodeOf(err); code == kernel.ErrKnowledgeRefUnresolved ||
				code == kernel.ErrVersionUnresolved {
				return "", nil
			}
			return "", err
		}
		if resolution.Status != knowledge.StatusResolved {
			return "", nil
		}
		return strings.TrimSpace(resolution.SchemaRef), nil
	}
	index, err := r.treeIndex()
	if err != nil {
		return "", err
	}
	unit, ok := index.Units[knowledge.AddressKey(op.Address)]
	if !ok {
		return "", nil
	}
	return strings.TrimSpace(unit.SchemaRef), nil
}

// definitionFor resolves one schema_ref against this ChangeSet's drafts or the
// fixed target basis. A ref naming another Repository is always rejected.
func (r *schemaResolver) definitionFor(ref string, drafts map[knowledge.ObjectID]knowledge.SchemaDefinition) (knowledge.SchemaDefinition, error) {
	if cached, ok := r.definitions[ref]; ok {
		return cached, nil
	}
	parsed, parsedOK := knowledge.ParseSchemaRef(ref)
	if !parsedOK {
		return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema_ref %q is not a schema/* object", ref)
	}
	if parsed.Repository != "" && parsed.Repository != r.target.ID() {
		return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
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
			return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
				"schema_ref %q commit does not exist", ref)
		}
		commit = parsed.Commit
	}
	value, err := r.schemaValue(parsed.Object, commit)
	if err != nil {
		return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"schema_ref %q does not resolve to a schema object", ref)
	}
	definition, err := knowledge.ParseSchemaDefinition(parsed.Object, value)
	if err != nil {
		return knowledge.SchemaDefinition{}, err
	}
	r.definitions[ref] = definition
	return definition, nil
}

// checkSchemaUpdate enforces that republishing a schema object ID is both
// contract-compatible and safe for the instances already published against it.
func (r *schemaResolver) checkSchemaUpdate(
	objectID knowledge.ObjectID,
	draft knowledge.SchemaDefinition,
	removedAddresses map[string]struct{},
	batchValues map[string]knowledge.Operation,
) error {
	current, readErr := r.schemaValue(objectID, r.at)
	if readErr != nil {
		if code := kernel.CodeOf(readErr); code == kernel.ErrKnowledgeRefUnresolved || code == kernel.ErrSchemaRevisionUnresolved {
			// First publication: nothing can reference it yet.
			return nil
		}
		return readErr
	}
	previous, parseErr := knowledge.ParseSchemaDefinition(objectID, current)
	if parseErr != nil {
		return parseErr
	}
	if breaking := knowledge.BreakingSchemaChanges(previous, draft); len(breaking) > 0 {
		return kernel.Fail(kernel.ErrSchemaIncompatible,
			"schema %s has breaking changes (%s); publish a new major schema object_id", objectID, strings.Join(breaking, "; "))
	}
	referrers, err := r.referrers(objectID)
	if err != nil {
		return err
	}
	for _, address := range referrers {
		key := knowledge.AddressKey(address)
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
		if err := knowledge.ValidateSchemaInstance(address, value, draft); err != nil {
			return kernel.Fail(kernel.ErrSchemaInstanceInvalid,
				"schema %s cannot be updated in place: %v", objectID, err)
		}
	}
	return nil
}

// checkSchemaRemoval refuses to delete a Domain Schema that instances still
// reference at the target basis.
func (r *schemaResolver) checkSchemaRemoval(objectID knowledge.ObjectID, removedAddresses map[string]struct{}) error {
	referrers, err := r.referrers(objectID)
	if err != nil {
		return err
	}
	remaining := []string{}
	for _, address := range referrers {
		if _, removed := removedAddresses[knowledge.AddressKey(address)]; removed {
			continue
		}
		remaining = append(remaining, knowledge.AddressKey(address))
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
// answer from their own index; tree providers reuse the decoded tree.
func (r *schemaResolver) referrers(schema knowledge.ObjectID) ([]knowledge.Address, error) {
	if r.nativeOK {
		locator, ok := r.native.(knowledge.SchemaReferrerLocator)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s cannot prove which instances reference schema %s", r.target.ID(), schema)
		}
		return locator.SchemaReferrerAddresses(schema, r.at)
	}
	index, err := r.treeIndex()
	if err != nil {
		return nil, err
	}
	addresses := []knowledge.Address{}
	for _, unit := range index.Units {
		if knowledge.IsSchemaObject(unit.Address.ObjectID) {
			continue
		}
		if parsed, ok := knowledge.ParseSchemaRef(unit.SchemaRef); ok && parsed.Object == schema {
			addresses = append(addresses, unit.Address)
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		return knowledge.AddressKey(addresses[i]) < knowledge.AddressKey(addresses[j])
	})
	return addresses, nil
}

// unitValue reads one already-published unit at the target basis. Binding units
// have no inline Snapshot value and report false.
func (r *schemaResolver) unitValue(address knowledge.Address) (any, bool, error) {
	if r.nativeOK {
		resolution, err := r.native.ResolveAddress(address, r.at)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil, false, nil
			}
			return nil, false, err
		}
		if resolution.Status != knowledge.StatusResolved {
			return nil, false, nil
		}
		if resolution.ValueSource != nil && resolution.ValueSource.Kind == knowledge.ValueSourceBinding {
			return nil, false, nil
		}
		value, err := r.native.ReadAddress(address, r.at)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil, false, nil
			}
			return nil, false, err
		}
		return value.Value, true, nil
	}
	index, err := r.treeIndex()
	if err != nil {
		return nil, false, err
	}
	unit, ok := index.Units[knowledge.AddressKey(address)]
	if !ok {
		return nil, false, nil
	}
	if unit.ValueSource != nil && unit.ValueSource.Kind == knowledge.ValueSourceBinding {
		return nil, false, nil
	}
	return unit.Value, true, nil
}
