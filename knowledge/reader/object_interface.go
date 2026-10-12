package reader

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

// ResolvedInterface is one ordinary implementation Member and all contracts
// resolved at immutable, explicitly available repository coordinates.
type ResolvedInterface struct {
	Repository                          kernel.RepositoryID                   `json:"repository"`
	Commit                              kernel.CommitID                       `json:"commit"`
	Address                             knowledge.Address                     `json:"address"`
	DeclarationDigest                   kernel.Digest                         `json:"declarationDigest"`
	Contract                            knowledge.PinnedKnowledgeRef          `json:"contract"`
	ContractDigest                      kernel.Digest                         `json:"contractDigest"`
	Definition                          knowledge.InterfaceDefinition         `json:"definition"`
	ImplementationSchema                knowledge.PinnedKnowledgeRef          `json:"implementationSchema"`
	ImplementationSchemaDigest          kernel.Digest                         `json:"implementationSchemaDigest"`
	ImplementationSchemaStructureDigest kernel.Digest                         `json:"implementationSchemaStructureDigest"`
	Implementation                      map[string]any                        `json:"implementation"`
	InputSchemas                        map[string]knowledge.SchemaDefinition `json:"inputSchemas,omitempty"`
	ResultSchemas                       map[string]knowledge.SchemaDefinition `json:"resultSchemas,omitempty"`
}

func (r *Reader) ResolveObjectInterface(repository kernel.RepositoryID, commit kernel.CommitID, object knowledge.ObjectID, member string) (ResolvedInterface, error) {
	repo, err := r.repoByID(repository)
	if err != nil {
		return ResolvedInterface{}, err
	}
	return ResolveRepoObjectInterface(repo, commit, object, member)
}

// ResolveRepoObjectInterface is declaration-only. Repository access cannot
// infer a foreign repository; use an explicitly pinned Serving for federation.
func ResolveRepoObjectInterface(repo knowledge.Repository, commit kernel.CommitID, object knowledge.ObjectID, member string) (ResolvedInterface, error) {
	return resolveObjectInterface(interfaceResolver{repository: repo.ID(), commit: commit, lookup: func(id kernel.RepositoryID, at kernel.CommitID) (knowledge.Repository, error) {
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
func (s *Serving) ResolveObjectInterfaceAt(repository kernel.RepositoryID, object knowledge.ObjectID, member string) (ResolvedInterface, error) {
	repo, err := s.Member(repository)
	if err != nil {
		return ResolvedInterface{}, err
	}
	resolver := interfaceResolver{repository: repository, commit: s.pin.Repositories[repository], pins: s.pin.Repositories, lookup: func(id kernel.RepositoryID, at kernel.CommitID) (knowledge.Repository, error) {
		if s.pin.Repositories[id] != at {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface dependency is outside dataset release")
		}
		return s.Member(id)
	}}
	return resolveObjectInterface(resolver, repo, object, member)
}

// DiscoverObjectInterfaces reads only a named object's units and the bounded
// dependencies declared by its implementations. It never scans repositories.
func (s *Serving) DiscoverObjectInterfaces(object knowledge.ObjectID) ([]ResolvedInterface, error) {
	out := []ResolvedInterface{}
	err := s.eachObject(object, func(repository kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		value, err := repo.Read(object, commit)
		if err != nil {
			return err
		}
		for _, address := range value.Units {
			if address.AspectName != knowledge.ObjectInterfacesAspect || address.Kind != knowledge.KindMember {
				continue
			}
			resolved, err := s.ResolveObjectInterfaceAt(repository, object, address.MemberKey)
			if err != nil {
				return err
			}
			out = append(out, resolved)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repository != out[j].Repository {
			return out[i].Repository < out[j].Repository
		}
		return out[i].Address.MemberKey < out[j].Address.MemberKey
	})
	return out, err
}
