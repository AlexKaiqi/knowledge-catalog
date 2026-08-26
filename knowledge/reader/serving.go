package reader

import (
	"kc/kernel"
	"kc/knowledge"
)

// MemberLookup mounts a Snapshot by id. Catalog.Require is the usual implementation.
type MemberLookup func(kernel.RepositoryID) (knowledge.Repository, error)

// WorkspacePin is a consumer read basis: one ResolveWorkspace result, frozen for the command.
// Catalog produces the coordinates; this package reads object_id at those commits.

type WorkspacePin struct {
	WorkspaceID  string                                  `json:"workspaceId"`
	Revision     int                                     `json:"revision"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
}

// Serving is the consumer read face on one WorkspacePin.
// Callers name a Workspace (CLI --workspace). They do not pass repository, ref, or commit.

type Serving struct {
	lookup MemberLookup
	pin    WorkspacePin
}

// FederatedValue is one member hit of a consumer read.
// Same field set as knowledge.KnowledgeValue so read --workspace and search --workspace
// share one envelope. objectId is kept for checkout / older callers.

type FederatedValue struct {
	KnowledgeRef knowledge.KnowledgeRef        `json:"knowledgeRef"`
	Repository   kernel.RepositoryID           `json:"repository"`
	Commit       kernel.CommitID               `json:"commit"`
	ObjectID     knowledge.ObjectID            `json:"objectId"`
	Address      knowledge.Address             `json:"address"`
	Value        any                           `json:"value"`
	Provenance   *knowledge.ProvenanceEnvelope `json:"provenance,omitempty"`
}

func federatedOf(repositoryID kernel.RepositoryID, commit kernel.CommitID, objectID knowledge.ObjectID, src knowledge.KnowledgeValue, assembled any) FederatedValue {
	addr := src.Address
	if addr.ObjectID == "" {
		addr = knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}
	}
	return FederatedValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repositoryID, Object: objectID},
		Repository:   repositoryID,
		Commit:       commit,
		ObjectID:     objectID,
		Address:      addr,
		Value:        assembled,
		Provenance:   src.Provenance,
	}
}

// ObjectLog is object history on one repository at the serving pin. Not git log.

type ObjectLog struct {
	Repository kernel.RepositoryID        `json:"repository"`
	ObjectID   knowledge.ObjectID         `json:"objectId"`
	Commit     kernel.CommitID            `json:"commit"`
	Revisions  []knowledge.ObjectRevision `json:"revisions"`
}

func Open(lookup MemberLookup, pin WorkspacePin) *Serving {
	return &Serving{lookup: lookup, pin: pin}
}

func FederatedRead(lookup MemberLookup, pin WorkspacePin, objectID knowledge.ObjectID) ([]FederatedValue, error) {
	return Open(lookup, pin).Read(objectID, nil)
}

func (s *Serving) Pin() WorkspacePin { return s.pin }

func (s *Serving) Read(objectID knowledge.ObjectID, selector *knowledge.AspectSelector) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		value, err := repo.Read(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		v := value.Value
		if selector != nil {
			v = knowledge.SelectAspects(value.Value, value.Units, selector)
		}
		out = append(out, federatedOf(repositoryID, commit, objectID, value, v))
		return nil
	})
	return out, err
}

func (s *Serving) ReadAddress(address knowledge.Address) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		value, err := repo.ReadAddress(address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, federatedOf(repositoryID, commit, address.ObjectID, value, value.Value))
		return nil
	})
	return out, err
}

func (s *Serving) Resolve(objectID knowledge.ObjectID) ([]knowledge.Resolution, error) {
	out := []knowledge.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		resolution, err := repo.Resolve(objectID, commit)
		if err != nil {
			return err
		}
		if resolution.Status == knowledge.StatusUnresolved {
			return nil
		}
		out = append(out, resolution)
		return nil
	})
	return out, err
}

// ResolveAddress resolves one exact Aspect/Member unit at every repository in
// the pinned Workspace. It is the Address-level counterpart of Resolve and
// keeps connector/maintenance callers from accidentally comparing an
// assembled Entity digest with a unit digest.
func (s *Serving) ResolveAddress(address knowledge.Address) ([]knowledge.Resolution, error) {
	out := []knowledge.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		resolution, err := repo.ResolveAddress(address, commit)
		if err != nil {
			return err
		}
		if resolution.Status == knowledge.StatusUnresolved {
			return nil
		}
		out = append(out, resolution)
		return nil
	})
	return out, err
}

// ResolveBinding resolves one exact Aspect Binding against every pinned
// Workspace member. Missing addresses are ignored; malformed declarations
// fail closed and are never treated as Snapshot values.
func (s *Serving) ResolveBinding(address knowledge.Address) ([]ResolvedBinding, error) {
	out := []ResolvedBinding{}
	err := s.eachRepository(func(_ kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		resolution, err := repo.ResolveAddress(address, commit)
		if err != nil {
			return err
		}
		if resolution.Status != knowledge.StatusResolved {
			return nil
		}
		binding, err := ResolveRepoBinding(repo, commit, address)
		if err != nil {
			return err
		}
		out = append(out, binding)
		return nil
	})
	return out, err
}

func (s *Serving) List() ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		values, err := repo.List(commit)
		if err != nil {
			return err
		}
		for _, value := range values {
			out = append(out, federatedOf(repositoryID, commit, value.Address.ObjectID, value, value.Value))
		}
		return nil
	})
	return out, err
}

// Relations finds Canonical Relations touching one endpoint across the fixed
// Workspace pin. The reference implementation scans each member; a derived
// endpoint projection may accelerate candidate discovery without changing the
// returned basis or body.
func (s *Serving) Relations(query RelationQuery) ([]RelationHit, error) {
	out := []RelationHit{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		hits, err := relationsAt(repo, repositoryID, commit, query)
		if err != nil {
			return err
		}
		out = append(out, hits...)
		return nil
	})
	sortRelationHits(out)
	return out, err
}

func (s *Serving) GetProvenance(objectID knowledge.ObjectID) ([]knowledge.ProvenanceTrace, error) {
	out := []knowledge.ProvenanceTrace{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		trace, err := repo.GetProvenance(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		out = append(out, trace)
		return nil
	})
	return out, err
}

func (s *Serving) Log(objectID knowledge.ObjectID, limit int) ([]ObjectLog, error) {
	out := []ObjectLog{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		revs, err := repo.Log(objectID, commit, limit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		if len(revs) == 0 {
			return nil
		}
		out = append(out, ObjectLog{
			Repository: repositoryID,
			ObjectID:   objectID,
			Commit:     commit,
			Revisions:  revs,
		})
		return nil
	})
	return out, err
}

func (s *Serving) DescribeSchema(objectID knowledge.ObjectID) ([]SchemaReport, error) {
	out := []SchemaReport{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo knowledge.Repository) error {
		report, err := DescribeRepoSchema(repo, commit, objectID)
		if err != nil {
			return err
		}
		out = append(out, report)
		return nil
	})
	return out, err
}

func (s *Serving) eachRepository(fn func(kernel.RepositoryID, kernel.CommitID, knowledge.Repository) error) error {
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Repositories))
	for id := range s.pin.Repositories {
		ids = append(ids, id)
	}
	sortRepoIDs(ids)
	for _, repositoryID := range ids {
		commit := s.pin.Repositories[repositoryID]
		repo, err := s.lookup(repositoryID)
		if err != nil {
			return err
		}
		if err := fn(repositoryID, commit, repo); err != nil {
			return err
		}
	}
	return nil
}

func sortRepoIDs(ids []kernel.RepositoryID) {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}
