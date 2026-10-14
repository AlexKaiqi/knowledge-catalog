package liveaccess

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

// ResolvedBinding is the complete stable access declaration resolved at one
// Repository commit. Credentials and observation values stay out of it.
// Origin comes from the Domain Schema that names the Aspect.
type ResolvedBinding struct {
	Repository        kernel.RepositoryID                     `json:"repository"`
	DeclarationCommit kernel.CommitID                         `json:"declarationCommit"`
	Address           objectmodel.Address                     `json:"address"`
	DeclarationDigest kernel.Digest                           `json:"declarationDigest"`
	Mode              objectmodel.BindingMode                 `json:"mode"`
	Runtime           string                                  `json:"runtime"`
	Protocol          string                                  `json:"protocol"`
	Operations        map[string]objectmodel.BindingOperation `json:"operations"`
	SchemaRef         string                                  `json:"schemaRef,omitempty"`
	Origin            string                                  `json:"origin,omitempty"`
	DescriptorRef     objectmodel.ObjectID                    `json:"descriptorRef,omitempty"`
	DescriptorDigest  kernel.Digest                           `json:"descriptorDigest,omitempty"`
}

func ResolveRepoBinding(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (ResolvedBinding, error) {
	if err := validateBindingAddress(address); err != nil {
		return ResolvedBinding{}, err
	}
	binding, err := resolveSchemaBinding(repo, commit, address)
	if err == nil {
		return binding, nil
	}
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		return ResolvedBinding{}, err
	}
	instance, instErr := resolveInstanceBinding(repo, commit, address)
	if instErr == nil {
		return instance, nil
	}
	if kernel.CodeOf(instErr) == kernel.ErrObjectRefUnresolved {
		return ResolvedBinding{}, err
	}
	return ResolvedBinding{}, instErr
}

func validateBindingAddress(address objectmodel.Address) error {
	if strings.TrimSpace(address.AspectName) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "resolve-binding requires --aspect")
	}
	if objectmodel.IsSchemaObject(address.ObjectID) {
		return kernel.Fail(kernel.ErrUsageInvalid, "Bound State access targets an entity, not a schema object")
	}
	return nil
}

func resolveSchemaBinding(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (ResolvedBinding, error) {
	schema, err := boundSchemaFor(repo, commit, address)
	if err != nil {
		return ResolvedBinding{}, err
	}
	return resolvedSchemaBinding(repo.ID(), commit, address, schema), nil
}

func resolvedSchemaBinding(repository kernel.RepositoryID, commit kernel.CommitID, address objectmodel.Address, schema objectmodel.SchemaDefinition) ResolvedBinding {
	source := schema.BindingSource()
	return ResolvedBinding{
		Repository:        repository,
		DeclarationCommit: commit,
		Address:           address,
		DeclarationDigest: objectmodel.BoundDeclarationDigest(schema),
		Mode:              source.Binding.Mode,
		Runtime:           source.Binding.Runtime,
		Protocol:          source.Binding.Protocol,
		Operations:        copyOperations(source.Binding.Operations),
		SchemaRef:         string(schema.ObjectID),
		Origin:            schema.Origin,
	}
}

func resolveInstanceBinding(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (ResolvedBinding, error) {
	resolution, err := repo.ResolveAddress(address, commit)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if resolution.Status != objectmodel.StatusResolved {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "binding address %s is not resolved at %s", objectmodel.AddressKey(address), commit)
	}
	return resolveLegacyDeclaration(repo, commit, objectmodel.UnitDeclaration{Address: address, ValueSource: resolution.ValueSource, SchemaRef: resolution.SchemaRef, DeclarationDigest: resolution.DeclarationDigest})
}

// ResolveRepoDeclarationBinding adopts one complete unit declaration already
// selected by Object's fixed read plan. Auxiliary reads stay on the caller's
// scoped repository; the object and unselected siblings are not loaded again.
func ResolveRepoDeclarationBinding(repo objectmodel.Repository, commit kernel.CommitID, declaration objectmodel.UnitDeclaration) (ResolvedBinding, error) {
	if err := validateBindingAddress(declaration.Address); err != nil {
		return ResolvedBinding{}, err
	}
	if err := objectmodel.AssertWritable(declaration.Address); err != nil {
		return ResolvedBinding{}, err
	}
	if declaration.SchemaRef != "" {
		ref, ok := objectmodel.ParseSchemaRef(declaration.SchemaRef)
		if !ok || (ref.Repository != "" && ref.Repository != repo.ID()) {
			return ResolvedBinding{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "selected Binding has an invalid adopted Schema")
		}
		at := commit
		if ref.Commit != "" {
			at = ref.Commit
		}
		value, err := repo.Read(ref.Object, at)
		if err != nil {
			return ResolvedBinding{}, err
		}
		schema, err := objectmodel.ParseSchemaDefinition(ref.Object, value.Value)
		if err != nil {
			return ResolvedBinding{}, err
		}
		if schema.Bound() {
			if schema.Aspect != declaration.Address.AspectName {
				return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "selected Binding Schema declares a different Aspect")
			}
			return resolvedSchemaBinding(repo.ID(), commit, declaration.Address, schema), nil
		}
	}
	return resolveLegacyDeclaration(repo, commit, declaration)
}

func resolveLegacyDeclaration(repo objectmodel.Repository, commit kernel.CommitID, declaration objectmodel.UnitDeclaration) (ResolvedBinding, error) {
	address := declaration.Address
	source := declaration.ValueSource
	if source == nil || source.Kind != objectmodel.ValueSourceBinding || source.Binding == nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has no Bound State schema origin", objectmodel.AddressKey(address))
	}
	if err := objectmodel.ValidateValueSource(source); err != nil {
		return ResolvedBinding{}, err
	}
	binding := source.Binding
	out := ResolvedBinding{
		Repository:        repo.ID(),
		DeclarationCommit: commit,
		Address:           address,
		DeclarationDigest: declaration.DeclarationDigest,
		Mode:              binding.Mode,
		Runtime:           binding.Runtime,
		Protocol:          binding.Protocol,
		Operations:        copyOperations(binding.Operations),
		SchemaRef:         declaration.SchemaRef,
		DescriptorRef:     binding.DescriptorRef,
	}
	if declaration.SchemaRef != "" {
		origin, err := schemaAccessOrigin(repo, commit, declaration.SchemaRef)
		if err != nil && kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			return ResolvedBinding{}, err
		}
		if err == nil {
			out.Origin = origin
		}
	}
	if binding.DescriptorRef == "" {
		return out, nil
	}
	descriptor, err := repo.Read(binding.DescriptorRef, commit)
	if err != nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "descriptor %s is not resolved at %s", binding.DescriptorRef, commit)
	}
	value, ok := descriptor.Value.(map[string]any)
	if ok && value["kind"] != "ResourceDescriptor" {
		if definition, nested := value["definition"].(map[string]any); nested {
			value = definition
		}
	}
	if !ok || value["kind"] != "ResourceDescriptor" {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "descriptor %s must be a ResourceDescriptor", binding.DescriptorRef)
	}
	out.Runtime = strings.TrimSpace(asString(value["runtime"]))
	out.Protocol = strings.TrimSpace(asString(value["protocol"]))
	out.Operations, err = descriptorOperations(value["access"])
	if err != nil {
		return ResolvedBinding{}, err
	}
	if out.Runtime == "" || out.Protocol == "" || len(out.Operations) == 0 {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires runtime, protocol and access", binding.DescriptorRef)
	}
	out.DescriptorDigest = kernel.CanonicalDigest(value)
	return out, nil
}

// BoundAspectDeclarations returns State handles implied by Domain Schema
// origin for one existing object, excluding Aspects already stored as units.
func BoundAspectDeclarations(repo objectmodel.Repository, commit kernel.CommitID, objectID objectmodel.ObjectID, existing []objectmodel.UnitDeclaration) ([]objectmodel.UnitDeclaration, error) {
	if objectmodel.IsSchemaObject(objectID) {
		return nil, nil
	}
	schemas, err := listBoundSchemas(repo, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return nil, nil
		}
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, nil
	}
	entity, err := objectEntity(repo, commit, objectID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, declaration := range existing {
		if declaration.Address.AspectName != "" {
			seen[declaration.Address.AspectName] = struct{}{}
		}
	}
	out := []objectmodel.UnitDeclaration{}
	for _, schema := range schemas {
		if _, exists := seen[schema.Aspect]; exists {
			continue
		}
		if entity != "" && schema.Entity != entity {
			continue
		}
		source := schema.BindingSource()
		address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectID, AspectName: schema.Aspect}
		out = append(out, objectmodel.UnitDeclaration{
			Address: address, SchemaRef: string(schema.ObjectID), ValueSource: source,
			DeclarationDigest: objectmodel.BoundDeclarationDigest(schema),
		})
		seen[schema.Aspect] = struct{}{}
	}
	return out, nil
}

func boundSchemaFor(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address) (objectmodel.SchemaDefinition, error) {
	schemas, err := listBoundSchemas(repo, commit)
	if err != nil {
		return objectmodel.SchemaDefinition{}, err
	}
	entity, err := objectEntity(repo, commit, address.ObjectID)
	if err != nil {
		return objectmodel.SchemaDefinition{}, err
	}
	matches := []objectmodel.SchemaDefinition{}
	for _, schema := range schemas {
		if schema.Aspect != address.AspectName {
			continue
		}
		if entity != "" && schema.Entity != entity {
			continue
		}
		matches = append(matches, schema)
	}
	if len(matches) == 0 {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"no Bound State schema origin for %s aspect %s", address.ObjectID, address.AspectName)
	}
	if len(matches) > 1 {
		return objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrUsageInvalid,
			"multiple Bound State schemas declare aspect %s", address.AspectName)
	}
	return matches[0], nil
}

func listBoundSchemas(repo objectmodel.Repository, commit kernel.CommitID) ([]objectmodel.SchemaDefinition, error) {
	var ids []objectmodel.ObjectID
	var err error
	if locator, ok := repo.(objectmodel.BindingLocator); ok {
		ids, err = locator.BindingSchemaObjectIDs(commit)
	} else if store, ok := repo.(objectmodel.SchemaStore); ok {
		ids, err = store.SchemaObjectIDs(commit)
	} else {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not provide schema namespace location", repo.ID())
	}
	if err != nil {
		return nil, err
	}
	out := []objectmodel.SchemaDefinition{}
	for _, id := range ids {
		value, err := repo.Read(id, commit)
		if err != nil {
			return nil, err
		}
		definition, err := objectmodel.ParseSchemaDefinition(id, value.Value)
		if err != nil {
			return nil, err
		}
		if definition.Bound() {
			out = append(out, definition)
		}
	}
	return out, nil
}

func objectEntity(repo objectmodel.Repository, commit kernel.CommitID, objectID objectmodel.ObjectID) (string, error) {
	locator, ok := repo.(objectmodel.SchemaAdoptionLocator)
	if !ok {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "virtual Bound Aspect requires fixed Schema adoption metadata")
	}
	refs, err := locator.AdoptedSchemaRefs(objectID, commit)
	if err != nil {
		return "", err
	}
	seen := map[string]struct{}{}
	for _, reference := range refs {
		ref := strings.TrimSpace(reference)
		if ref == "" {
			continue
		}
		parsed, ok := objectmodel.ParseSchemaRef(ref)
		if !ok || parsed.Repository != "" && parsed.Repository != repo.ID() {
			return "", kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "object has an invalid adopted Schema reference")
		}
		at := commit
		if parsed.Commit != "" {
			at = parsed.Commit
		}
		schemaValue, err := repo.Read(parsed.Object, at)
		if err != nil {
			return "", err
		}
		definition, err := objectmodel.ParseSchemaDefinition(parsed.Object, schemaValue.Value)
		if err != nil {
			return "", err
		}
		if definition.Entity != "" {
			seen[definition.Entity] = struct{}{}
		}
	}
	if len(seen) != 1 {
		return "", nil
	}
	for entity := range seen {
		return entity, nil
	}
	return "", nil
}

func copyOperations(source map[string]objectmodel.BindingOperation) map[string]objectmodel.BindingOperation {
	out := make(map[string]objectmodel.BindingOperation, len(source))
	for name, operation := range source {
		out[name] = operation
	}
	return out
}

func descriptorOperations(raw any) (map[string]objectmodel.BindingOperation, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor access must be an object")
	}
	out := map[string]objectmodel.BindingOperation{}
	for name, rawOperation := range obj {
		operation, ok := rawOperation.(map[string]any)
		if !ok {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operation %s must be an object", name)
		}
		call := strings.TrimSpace(asString(operation["call"]))
		if strings.TrimSpace(name) == "" || call == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operations require non-empty names and calls")
		}
		out[name] = objectmodel.BindingOperation{Call: call}
	}
	return out, nil
}

// asString reads an optional string field from a decoded declaration without
// asserting on shape; callers validate the resulting declaration.
func asString(value any) string {
	s, _ := value.(string)
	return s
}
