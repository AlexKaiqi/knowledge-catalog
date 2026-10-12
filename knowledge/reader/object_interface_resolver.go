package reader

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

type interfaceResolver struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	pins       map[kernel.RepositoryID]kernel.CommitID
	lookup     func(kernel.RepositoryID, kernel.CommitID) (knowledge.Repository, error)
}

func resolveObjectInterface(r interfaceResolver, repo knowledge.Repository, object knowledge.ObjectID, member string) (ResolvedInterface, error) {
	if object == "" || strings.TrimSpace(member) == "" || r.commit == "" {
		return ResolvedInterface{}, kernel.Fail(kernel.ErrUsageInvalid, "interface resolution requires object, implementation and fixed commit")
	}
	address := knowledge.Address{Kind: knowledge.KindMember, ObjectID: object, AspectName: knowledge.ObjectInterfacesAspect, MemberKey: member}
	value, err := repo.ReadAddress(address, r.commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	declaration, err := repo.ResolveAddress(address, r.commit)
	if err != nil {
		return ResolvedInterface{}, err
	}
	if declaration.Status != knowledge.StatusResolved {
		return ResolvedInterface{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface implementation is not resolved at its fixed basis")
	}
	implementation, err := knowledge.ParseInterfaceImplementation(value.Value)
	if err != nil {
		return ResolvedInterface{}, err
	}
	schemaRef, schema, err := r.schema(r.repository, r.commit, declaration.SchemaRef)
	if err != nil {
		return ResolvedInterface{}, err
	}
	if err = knowledge.ValidateSchemaInstance(address, value.Value, schema); err != nil {
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
	definition, err := knowledge.ParseInterfaceDefinition(contractRef.Object, contractValue.Value)
	if err != nil {
		return ResolvedInterface{}, err
	}
	out := ResolvedInterface{Repository: r.repository, Commit: r.commit, Address: address, DeclarationDigest: kernel.CanonicalDigest(map[string]any{"value": value.Value, "schemaRef": declaration.SchemaRef}), Contract: contractRef, ContractDigest: definition.Digest, Definition: definition, ImplementationSchema: schemaRef, ImplementationSchemaDigest: kernel.CanonicalDigest(schema), ImplementationSchemaStructureDigest: knowledge.SchemaStructureDigest(schema), Implementation: implementation.Value, InputSchemas: map[string]knowledge.SchemaDefinition{}, ResultSchemas: map[string]knowledge.SchemaDefinition{}}
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

func (r interfaceResolver) schema(repository kernel.RepositoryID, commit kernel.CommitID, ref string) (knowledge.PinnedKnowledgeRef, knowledge.SchemaDefinition, error) {
	parsed, ok := knowledge.ParseSchemaRef(ref)
	if !ok || (parsed.Repository != "" && parsed.Repository != repository) {
		return knowledge.PinnedKnowledgeRef{}, knowledge.SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "interface schema_ref %q must identify a same-repository schema", ref)
	}
	if parsed.Commit != "" {
		commit = parsed.Commit
	}
	repo, err := r.lookup(repository, commit)
	if err != nil {
		return knowledge.PinnedKnowledgeRef{}, knowledge.SchemaDefinition{}, err
	}
	value, err := repo.Read(parsed.Object, commit)
	if err != nil {
		return knowledge.PinnedKnowledgeRef{}, knowledge.SchemaDefinition{}, err
	}
	schema, err := knowledge.ParseSchemaDefinition(parsed.Object, value.Value)
	return knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: parsed.Object}, Commit: commit}, schema, err
}

func (r interfaceResolver) reference(ref string) (knowledge.PinnedKnowledgeRef, error) {
	if !strings.HasPrefix(ref, "kc://") {
		if strings.TrimSpace(ref) == "" || strings.Contains(ref, "@") {
			return knowledge.PinnedKnowledgeRef{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid interfaceRef %q", ref)
		}
		return knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: r.repository, Object: knowledge.ObjectID(ref)}, Commit: r.commit}, nil
	}
	refs := r.pins
	if refs == nil {
		refs = map[kernel.RepositoryID]kernel.CommitID{r.repository: r.commit}
	}
	for repository, commit := range refs {
		prefix := knowledge.FormatPinnedRef(repository, commit, "")
		if strings.HasPrefix(ref, prefix) && len(ref) > len(prefix) {
			return knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: knowledge.ObjectID(strings.TrimPrefix(ref, prefix))}, Commit: commit}, nil
		}
	}
	return knowledge.PinnedKnowledgeRef{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interfaceRef %q is not explicitly available at this pinned scope", ref)
}
