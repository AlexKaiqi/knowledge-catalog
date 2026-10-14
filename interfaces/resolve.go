package interfaces

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

type interfaceResolver struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	pins       map[kernel.RepositoryID]kernel.CommitID
	lookup     func(kernel.RepositoryID, kernel.CommitID) (objectmodel.Repository, error)
}

func resolveObjectInterface(r interfaceResolver, repo objectmodel.Repository, object objectmodel.ObjectID, member string) (ResolvedInterface, error) {
	if object == "" || strings.TrimSpace(member) == "" || r.commit == "" {
		return ResolvedInterface{}, kernel.Fail(kernel.ErrUsageInvalid, "interface resolution requires object, implementation and fixed commit")
	}
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: object, AspectName: ObjectInterfacesAspect, MemberKey: member}
	value, err := repo.ReadAddress(address, r.commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	declaration, err := repo.ResolveAddress(address, r.commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	if declaration.Status != objectmodel.StatusResolved {
		return ResolvedInterface{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface implementation is not resolved at its fixed basis")
	}
	implementation, err := ParseInterfaceImplementation(value.Value)
	if err != nil {
		return ResolvedInterface{}, err
	}
	schemaRef, schema, err := r.schema(r.repository, r.commit, declaration.SchemaRef)
	if err != nil {
		return ResolvedInterface{}, err
	}
	if err = objectmodel.ValidateSchemaInstance(address, value.Value, schema); err != nil {
		return ResolvedInterface{}, err
	}
	contractRef, err := r.reference(implementation.InterfaceRef)
	if err != nil {
		return ResolvedInterface{}, err
	}
	contractRepo, err := r.lookup(contractRef.Repository, contractRef.Commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	contractValue, err := contractRepo.Read(contractRef.Object, contractRef.Commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	definition, err := ParseInterfaceDefinition(contractRef.Object, contractValue.Value)
	if err != nil {
		return ResolvedInterface{}, err
	}
	out := ResolvedInterface{Repository: r.repository, Commit: r.commit, Address: address, DeclarationDigest: kernel.CanonicalDigest(map[string]any{"value": value.Value, "schemaRef": declaration.SchemaRef}), Contract: contractRef, ContractDigest: definition.Digest, Definition: definition, ImplementationSchema: schemaRef, ImplementationSchemaDigest: kernel.CanonicalDigest(schema), ImplementationSchemaStructureDigest: objectmodel.SchemaStructureDigest(schema), Implementation: implementation.Value, InputSchemas: map[string]objectmodel.SchemaDefinition{}, ResultSchemas: map[string]objectmodel.SchemaDefinition{}}
	for name, operation := range definition.Operations {
		if operation.InputSchemaRef != "" {
			_, input, err := r.schema(contractRef.Repository, contractRef.Commit, operation.InputSchemaRef)
			if err != nil {
				return ResolvedInterface{}, err
			}
			out.InputSchemas[name] = input
		}
		if operation.ResultSchemaRef != "" {
			_, result, err := r.schema(contractRef.Repository, contractRef.Commit, operation.ResultSchemaRef)
			if err != nil {
				return ResolvedInterface{}, err
			}
			out.ResultSchemas[name] = result
		}
	}
	return out, nil
}

func (r interfaceResolver) schema(repository kernel.RepositoryID, commit kernel.CommitID, ref string) (objectmodel.PinnedKnowledgeRef, objectmodel.SchemaDefinition, error) {
	parsed, ok := objectmodel.ParseSchemaRef(ref)
	if !ok || (parsed.Repository != "" && parsed.Repository != repository) {
		return objectmodel.PinnedKnowledgeRef{}, objectmodel.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "interface schema_ref %q must identify a same-repository schema", ref)
	}
	if parsed.Commit != "" {
		commit = parsed.Commit
	}
	repo, err := r.lookup(repository, commit)
	if err != nil {
		return objectmodel.PinnedKnowledgeRef{}, objectmodel.SchemaDefinition{}, err
	}
	value, err := repo.Read(parsed.Object, commit)
	if err != nil {
		return objectmodel.PinnedKnowledgeRef{}, objectmodel.SchemaDefinition{}, err
	}
	schema, err := objectmodel.ParseSchemaDefinition(parsed.Object, value.Value)
	return objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: repository, Object: parsed.Object}, Commit: commit}, schema, err
}

func (r interfaceResolver) reference(ref string) (objectmodel.PinnedKnowledgeRef, error) {
	if !strings.HasPrefix(ref, "kc://") {
		if strings.TrimSpace(ref) == "" || strings.Contains(ref, "@") {
			return objectmodel.PinnedKnowledgeRef{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid interfaceRef %q", ref)
		}
		return objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: r.repository, Object: objectmodel.ObjectID(ref)}, Commit: r.commit}, nil
	}
	refs := r.pins
	if refs == nil {
		refs = map[kernel.RepositoryID]kernel.CommitID{r.repository: r.commit}
	}
	for repository, commit := range refs {
		prefix := objectmodel.FormatPinnedRef(repository, commit, "")
		if strings.HasPrefix(ref, prefix) && len(ref) > len(prefix) {
			return objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: repository, Object: objectmodel.ObjectID(strings.TrimPrefix(ref, prefix))}, Commit: commit}, nil
		}
	}
	return objectmodel.PinnedKnowledgeRef{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interfaceRef %q is not explicitly available at this pinned scope", ref)
}
