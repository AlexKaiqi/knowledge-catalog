package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// CheckIndexDeclaration validates a selected declaration PUT with the same
// structural and Schema rules as Commit. Fields follow the input order. The
// plugin instance checks capability separately; this method never reserves a
// command, writes evidence, mutates Snapshot or promises storage admission or a
// future CAS result.
// Other operations in cs provide the same-batch Schema and final-unit context.
func (w *Writer) CheckIndexDeclaration(cs objectmodel.ChangeSet, address objectmodel.Address) (objectmodel.IndexDeclaration, []objectmodel.SchemaFieldDefinition, error) {
	if err := w.refuseRuntimeSystemMutation(cs.TargetRepository); err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	if err := validateChangeSet(cs); err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	target, err := w.store.Require(cs.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	if target.Archived() {
		return objectmodel.IndexDeclaration{}, nil, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", cs.TargetRepository)
	}
	if err := validateSchemaRefs(w.store, target, cs); err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	resolver, err := newSchemaResolver(target, cs)
	if err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	if err := resolver.prepareFinalUnits(cs); err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	drafts := map[objectmodel.ObjectID]objectmodel.SchemaDefinition{}
	selected := false
	for _, op := range cs.Operations {
		if op.Op != objectmodel.OpPut {
			continue
		}
		selected = selected || op.Address == address
		if objectmodel.IsSchemaObject(op.Address.ObjectID) {
			definition, err := objectmodel.ParseSchemaDefinition(op.Address.ObjectID, op.Value)
			if err != nil {
				return objectmodel.IndexDeclaration{}, nil, err
			}
			drafts[op.Address.ObjectID] = definition
		}
	}
	unit, found := resolver.final[objectmodel.AddressKey(address)]
	if !selected || !found || !objectmodel.IsIndexDeclarationRef(unit.SchemaRef) {
		return objectmodel.IndexDeclaration{}, nil, kernel.Fail(kernel.ErrUsageInvalid, "check requires a PUT adopting the index declaration Schema at %s", objectmodel.AddressKey(address))
	}
	return resolver.checkIndexDeclaration(w.store, unit, drafts)
}
