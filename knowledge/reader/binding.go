package reader

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// ResolvedBinding is the complete stable access declaration resolved at one
// Repository commit. Credentials and observation values stay out of it.
// Origin comes from the Domain Schema that names the Aspect.
type ResolvedBinding struct {
	Repository        kernel.RepositoryID                   `json:"repository"`
	DeclarationCommit kernel.CommitID                       `json:"declarationCommit"`
	Address           knowledge.Address                     `json:"address"`
	DeclarationDigest kernel.Digest                         `json:"declarationDigest"`
	Mode              knowledge.BindingMode                 `json:"mode"`
	Runtime           string                                `json:"runtime"`
	Protocol          string                                `json:"protocol"`
	Operations        map[string]knowledge.BindingOperation `json:"operations"`
	SchemaRef         string                                `json:"schemaRef,omitempty"`
	Origin            string                                `json:"origin,omitempty"`
	DescriptorRef     knowledge.ObjectID                    `json:"descriptorRef,omitempty"`
	DescriptorDigest  kernel.Digest                         `json:"descriptorDigest,omitempty"`
}

func (r *Reader) ResolveBinding(repositoryID kernel.RepositoryID, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	return ResolveRepoBinding(repo, commit, address)
}

func ResolveRepoBinding(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	if strings.TrimSpace(address.AspectName) == "" {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "resolve-binding requires --aspect")
	}
	if knowledge.IsSchemaObject(address.ObjectID) {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "Bound State access targets an entity, not a schema object")
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
	if kernel.CodeOf(instErr) == kernel.ErrKnowledgeRefUnresolved {
		return ResolvedBinding{}, err
	}
	return ResolvedBinding{}, instErr
}

func resolveSchemaBinding(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	if _, err := repo.Read(address.ObjectID, commit); err != nil {
		return ResolvedBinding{}, err
	}
	schema, err := boundSchemaFor(repo, commit, address)
	if err != nil {
		return ResolvedBinding{}, err
	}
	source := schema.BindingSource()
	return ResolvedBinding{
		Repository:        repo.ID(),
		DeclarationCommit: commit,
		Address:           address,
		DeclarationDigest: knowledge.BoundDeclarationDigest(schema),
		Mode:              source.Binding.Mode,
		Runtime:           source.Binding.Runtime,
		Protocol:          source.Binding.Protocol,
		Operations:        copyOperations(source.Binding.Operations),
		SchemaRef:         string(schema.ObjectID),
		Origin:            schema.Origin,
	}, nil
}

func resolveInstanceBinding(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	resolution, err := repo.ResolveAddress(address, commit)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if resolution.Status != knowledge.StatusResolved {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "binding address %s is not resolved at %s", knowledge.AddressKey(address), commit)
	}
	source := resolution.ValueSource
	if source == nil || source.Kind != knowledge.ValueSourceBinding || source.Binding == nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has no Bound State schema origin", knowledge.AddressKey(address))
	}
	if err := knowledge.ValidateValueSource(source); err != nil {
		return ResolvedBinding{}, err
	}
	binding := source.Binding
	out := ResolvedBinding{
		Repository:        repo.ID(),
		DeclarationCommit: commit,
		Address:           address,
		DeclarationDigest: resolution.DeclarationDigest,
		Mode:              binding.Mode,
		Runtime:           binding.Runtime,
		Protocol:          binding.Protocol,
		Operations:        copyOperations(binding.Operations),
		SchemaRef:         resolution.SchemaRef,
		DescriptorRef:     binding.DescriptorRef,
	}
	if resolution.SchemaRef != "" {
		origin, err := SchemaAccessOrigin(repo, commit, resolution.SchemaRef)
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
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "descriptor %s is not resolved at %s", binding.DescriptorRef, commit)
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
func BoundAspectDeclarations(repo knowledge.Repository, commit kernel.CommitID, objectID knowledge.ObjectID, existing []knowledge.UnitDeclaration) ([]knowledge.UnitDeclaration, error) {
	if knowledge.IsSchemaObject(objectID) {
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
	out := []knowledge.UnitDeclaration{}
	for _, schema := range schemas {
		if _, exists := seen[schema.Aspect]; exists {
			continue
		}
		if entity != "" && schema.Entity != entity {
			continue
		}
		source := schema.BindingSource()
		address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: objectID, AspectName: schema.Aspect}
		out = append(out, knowledge.UnitDeclaration{
			Address: address, SchemaRef: string(schema.ObjectID), ValueSource: source,
			DeclarationDigest: knowledge.BoundDeclarationDigest(schema),
		})
		seen[schema.Aspect] = struct{}{}
	}
	return out, nil
}

func boundSchemaFor(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (knowledge.SchemaDefinition, error) {
	schemas, err := listBoundSchemas(repo, commit)
	if err != nil {
		return knowledge.SchemaDefinition{}, err
	}
	entity, err := objectEntity(repo, commit, address.ObjectID)
	if err != nil {
		return knowledge.SchemaDefinition{}, err
	}
	matches := []knowledge.SchemaDefinition{}
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
		return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"no Bound State schema origin for %s aspect %s", address.ObjectID, address.AspectName)
	}
	if len(matches) > 1 {
		return knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrUsageInvalid,
			"multiple Bound State schemas declare aspect %s", address.AspectName)
	}
	return matches[0], nil
}

func listBoundSchemas(repo knowledge.Repository, commit kernel.CommitID) ([]knowledge.SchemaDefinition, error) {
	store, ok := repo.(knowledge.SchemaStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide schema namespace location", repo.ID())
	}
	ids, err := store.SchemaObjectIDs(commit)
	if err != nil {
		return nil, err
	}
	out := []knowledge.SchemaDefinition{}
	for _, id := range ids {
		value, err := repo.Read(id, commit)
		if err != nil {
			return nil, err
		}
		definition, err := knowledge.ParseSchemaDefinition(id, value.Value)
		if err != nil {
			return nil, err
		}
		if definition.Bound() {
			out = append(out, definition)
		}
	}
	return out, nil
}

func objectEntity(repo knowledge.Repository, commit kernel.CommitID, objectID knowledge.ObjectID) (string, error) {
	value, err := repo.Read(objectID, commit)
	if err != nil {
		return "", err
	}
	seen := map[string]struct{}{}
	for _, declaration := range value.Declarations {
		ref := strings.TrimSpace(declaration.SchemaRef)
		if ref == "" {
			continue
		}
		parsed, ok := knowledge.ParseSchemaRef(ref)
		if !ok {
			continue
		}
		at := commit
		if parsed.Commit != "" {
			at = parsed.Commit
		}
		schemaValue, err := repo.Read(parsed.Object, at)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			return "", err
		}
		definition, err := knowledge.ParseSchemaDefinition(parsed.Object, schemaValue.Value)
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

func copyOperations(source map[string]knowledge.BindingOperation) map[string]knowledge.BindingOperation {
	out := make(map[string]knowledge.BindingOperation, len(source))
	for name, operation := range source {
		out[name] = operation
	}
	return out
}

func descriptorOperations(raw any) (map[string]knowledge.BindingOperation, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor access must be an object")
	}
	out := map[string]knowledge.BindingOperation{}
	for name, rawOperation := range obj {
		operation, ok := rawOperation.(map[string]any)
		if !ok {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operation %s must be an object", name)
		}
		call := strings.TrimSpace(asString(operation["call"]))
		if strings.TrimSpace(name) == "" || call == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operations require non-empty names and calls")
		}
		out[name] = knowledge.BindingOperation{Call: call}
	}
	return out, nil
}

// asString reads an optional string field from a decoded declaration without
// asserting on shape; callers validate the resulting declaration.
func asString(value any) string {
	s, _ := value.(string)
	return s
}
