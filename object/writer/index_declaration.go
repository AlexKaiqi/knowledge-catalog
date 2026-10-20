package writer

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// validateIndexDeclarations checks every index declaration this ChangeSet
// produces: shape, and that each input locator resolves to a field on the
// Schema it names. It never checks capability; a well-shaped declaration that
// no plugin instance supports is still writable and is reported unsupported by
// the instance's manifest.
//
// A bare locator resolves in a fixed order: this ChangeSet's drafts, the
// target basis, then the DefaultRef head of every other Repository (the same
// order as descriptorRef). A Schema held by more than one other Repository is
// ambiguous and must be named with kc://<repo>/.
func (r *schemaResolver) validateIndexDeclarations(registry snapshot.Registry, cs objectmodel.ChangeSet, drafts map[objectmodel.ObjectID]objectmodel.SchemaDefinition) error {
	changed := map[string]struct{}{}
	for _, op := range cs.Operations {
		if op.Op == objectmodel.OpPut {
			changed[objectmodel.AddressKey(op.Address)] = struct{}{}
		}
	}
	for _, key := range sortedFinalKeys(r.final) {
		unit := r.final[key]
		if _, touched := changed[key]; !touched || !objectmodel.IsIndexDeclarationRef(unit.SchemaRef) {
			continue
		}
		if _, _, err := r.checkIndexDeclaration(registry, unit, drafts); err != nil {
			return err
		}
	}
	return nil
}

func (r *schemaResolver) checkIndexDeclaration(registry snapshot.Registry, unit objectmodel.Unit, drafts map[objectmodel.ObjectID]objectmodel.SchemaDefinition) (objectmodel.IndexDeclaration, []objectmodel.SchemaFieldDefinition, error) {
	key := objectmodel.AddressLabel(unit.Address)
	if unit.ValueSource != nil && unit.ValueSource.Kind != "" && unit.ValueSource.Kind != objectmodel.ValueSourceSnapshot {
		return objectmodel.IndexDeclaration{}, nil, kernel.Fail(kernel.ErrUsageInvalid,
			"index declaration %s cannot declare an external value source; a declaration is catalog data", key)
	}
	declaration, err := objectmodel.ParseIndexDeclaration(unit.Address, unit.Value)
	if err != nil {
		return objectmodel.IndexDeclaration{}, nil, err
	}
	fields := make([]objectmodel.SchemaFieldDefinition, 0, len(declaration.Inputs))
	for i, input := range declaration.Inputs {
		definition, err := r.locatorSchema(registry, input.Locator, drafts)
		if err != nil {
			return objectmodel.IndexDeclaration{}, nil, kernel.Fail(kernel.CodeOf(err), "index declaration %s inputs[%d].from %s: %s",
				key, i, input.From, err.Error())
		}
		field, found := schemaField(definition, input.Locator.Path)
		if !found {
			return objectmodel.IndexDeclaration{}, nil, kernel.Fail(kernel.ErrObjectRefUnresolved,
				"index declaration %s inputs[%d].from %s: schema %s has no field %s",
				key, i, input.From, definition.ObjectID, input.Locator.Path)
		}
		fields = append(fields, field)
	}
	return declaration, fields, nil
}

func (r *schemaResolver) locatorSchema(registry snapshot.Registry, locator objectmodel.FieldLocator, drafts map[objectmodel.ObjectID]objectmodel.SchemaDefinition) (objectmodel.SchemaDefinition, error) {
	ref := locator.Schema
	if ref.Repository == "" || ref.Repository == r.target.ID() {
		local := objectmodel.ParsedSchemaRef{Object: ref.Object, Commit: ref.Commit}
		definition, err := r.definitionFor(schemaRefString(local), drafts)
		if err == nil || ref.Repository != "" || ref.Commit != "" || kernel.CodeOf(err) != kernel.ErrSchemaRevisionUnresolved {
			return definition, err
		}
		return r.uniqueRemoteSchema(registry, ref.Object)
	}
	store, ok := registry.Get(ref.Repository)
	if !ok {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "repository %s is not available", ref.Repository)
	}
	commit := ref.Commit
	if commit == "" {
		head, err := store.Head(snapshot.DefaultRef)
		if err != nil {
			return objectmodel.SchemaDefinition{}, err
		}
		commit = head
	}
	return remoteSchema(store, ref.Object, commit)
}

func (r *schemaResolver) uniqueRemoteSchema(registry snapshot.Registry, id objectmodel.ObjectID) (objectmodel.SchemaDefinition, error) {
	ids := registry.IDs()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var holders []kernel.RepositoryID
	var found objectmodel.SchemaDefinition
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
			return objectmodel.SchemaDefinition{}, err
		}
		if head == "" {
			continue
		}
		definition, err := remoteSchema(store, id, head)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		}
		if err != nil {
			return objectmodel.SchemaDefinition{}, err
		}
		holders, found = append(holders, repoID), definition
	}
	switch len(holders) {
	case 0:
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "schema %s does not resolve", id)
	case 1:
		return found, nil
	default:
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrObjectIDConflict,
			"schema %s is held by several repositories (%v); name it as kc://<repo>/%s", id, holders, id)
	}
}

func remoteSchema(store snapshot.Store, id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.SchemaDefinition, error) {
	value, found, err := storeObjectValue(store, id, commit)
	if err != nil {
		return objectmodel.SchemaDefinition{}, err
	}
	if !found {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "schema %s does not resolve in %s", id, store.ID())
	}
	return objectmodel.ParseSchemaDefinition(id, value)
}

func schemaRefString(ref objectmodel.ParsedSchemaRef) string {
	if ref.Commit == "" {
		return string(ref.Object)
	}
	return string(ref.Object) + "@" + string(ref.Commit)
}

func schemaField(definition objectmodel.SchemaDefinition, path string) (objectmodel.SchemaFieldDefinition, bool) {
	for _, field := range definition.Fields {
		if field.Path == path {
			return field, true
		}
	}
	return objectmodel.SchemaFieldDefinition{}, false
}
