package interfaces

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// ResolvedInterface is one ordinary implementation Member and all contracts
// resolved at immutable, explicitly available repository coordinates.
type ResolvedInterface struct {
	Repository                          kernel.RepositoryID                     `json:"repository"`
	Commit                              kernel.CommitID                         `json:"commit"`
	Address                             objectmodel.Address                     `json:"address"`
	DeclarationDigest                   kernel.Digest                           `json:"declarationDigest"`
	Contract                            objectmodel.PinnedKnowledgeRef          `json:"contract"`
	ContractDigest                      kernel.Digest                           `json:"contractDigest"`
	Definition                          InterfaceDefinition                     `json:"definition"`
	ImplementationSchema                objectmodel.PinnedKnowledgeRef          `json:"implementationSchema"`
	ImplementationSchemaDigest          kernel.Digest                           `json:"implementationSchemaDigest"`
	ImplementationSchemaStructureDigest kernel.Digest                           `json:"implementationSchemaStructureDigest"`
	Implementation                      map[string]any                          `json:"implementation"`
	InputSchemas                        map[string]objectmodel.SchemaDefinition `json:"inputSchemas,omitempty"`
	ResultSchemas                       map[string]objectmodel.SchemaDefinition `json:"resultSchemas,omitempty"`
}

func ResolveObjectInterface(r *reader.Reader, repository kernel.RepositoryID, commit kernel.CommitID, object objectmodel.ObjectID, member string) (ResolvedInterface, error) {
	repo, err := r.Require(repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return ResolvedInterface{}, err
	}
	return ResolveRepoObjectInterface(repo, commit, object, member)
}

// ResolveRepoObjectInterface is declaration-only. Repository access cannot
// infer a foreign repository; use an explicitly pinned Serving for federation.
func ResolveRepoObjectInterface(repo objectmodel.Repository, commit kernel.CommitID, object objectmodel.ObjectID, member string) (ResolvedInterface, error) {
	return resolveObjectInterface(interfaceResolver{repository: repo.ID(), commit: commit, lookup: func(id kernel.RepositoryID, at kernel.CommitID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface dependency repository %s is not available", id)
		}
		if !repo.HasCommit(at) {
			return nil, kernel.Fail(kernel.ErrVersionUnresolved, "interface dependency commit %s is unavailable", at)
		}
		return repo, nil
	}}, repo, object, member)
}

// ResolveObjectInterfaceAt never obtains a raw repository view: every primary
// and secondary read uses the same pinned Dataset scope.
func ResolveObjectInterfaceAt(s *reader.Serving, repository kernel.RepositoryID, object objectmodel.ObjectID, member string) (ResolvedInterface, error) {
	repo, err := s.Member(repository)
	if err != nil {
		return ResolvedInterface{}, err
	}
	resolver := interfaceResolver{repository: repository, commit: s.Pin().Repositories[repository], pins: s.Pin().Repositories, lookup: func(id kernel.RepositoryID, at kernel.CommitID) (objectmodel.Repository, error) {
		if s.Pin().Repositories[id] != at {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface dependency is outside dataset release")
		}
		return s.Member(id)
	}}
	return resolveObjectInterface(resolver, repo, object, member)
}

// DiscoverObjectInterfaces reads the selected object's fixed declarations.
func DiscoverObjectInterfaces(s *reader.Serving, object objectmodel.ObjectID) ([]ResolvedInterface, error) {
	out := []ResolvedInterface{}
	values, err := s.Read(object, nil)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		for _, address := range value.Units {
			if address.AspectName != ObjectInterfacesAspect || address.Kind != objectmodel.KindMember {
				continue
			}
			resolved, err := ResolveObjectInterfaceAt(s, value.Repository, object, address.MemberKey)
			if err != nil {
				return nil, err
			}
			out = append(out, resolved)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repository != out[j].Repository {
			return out[i].Repository < out[j].Repository
		}
		return out[i].Address.MemberKey < out[j].Address.MemberKey
	})
	return out, nil
}
