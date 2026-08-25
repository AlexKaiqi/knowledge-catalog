package reader

import (
	"kc/kernel"
	"kc/repository"
)

// MemberLookup mounts a Snapshot by id. Catalog.Require is the usual implementation.
type MemberLookup func(kernel.RepositoryID) (repository.Repository, error)

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
// Same field set as repository.KnowledgeValue so read --workspace and search --workspace
// share one envelope. objectId is kept for checkout / older callers.

type FederatedValue struct {
	KnowledgeRef kernel.KnowledgeRef        `json:"knowledgeRef"`
	Repository   kernel.RepositoryID        `json:"repository"`
	Commit       kernel.CommitID            `json:"commit"`
	ObjectID     kernel.ObjectID            `json:"objectId"`
	Address      kernel.Address             `json:"address"`
	Value        any                        `json:"value"`
	Provenance   *kernel.ProvenanceEnvelope `json:"provenance,omitempty"`
}

func federatedOf(repositoryID kernel.RepositoryID, commit kernel.CommitID, objectID kernel.ObjectID, src repository.KnowledgeValue, assembled any) FederatedValue {
	addr := src.Address
	if addr.ObjectID == "" {
		addr = kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID}
	}
	return FederatedValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: repositoryID, Object: objectID},
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
	Repository kernel.RepositoryID         `json:"repository"`
	ObjectID   kernel.ObjectID             `json:"objectId"`
	Commit     kernel.CommitID             `json:"commit"`
	Revisions  []repository.ObjectRevision `json:"revisions"`
}

func Open(lookup MemberLookup, pin WorkspacePin) *Serving {
	return &Serving{lookup: lookup, pin: pin}
}

func FederatedRead(lookup MemberLookup, pin WorkspacePin, objectID kernel.ObjectID) ([]FederatedValue, error) {
	return Open(lookup, pin).Read(objectID, nil)
}

func (s *Serving) Pin() WorkspacePin { return s.pin }

func (s *Serving) Resolved() WorkspacePin { return s.pin }

func (s *Serving) Read(objectID kernel.ObjectID, selector *repository.AspectSelector) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		value, err := repo.Read(objectID, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return nil
			}
			return err
		}
		v := value.Value
		if selector != nil {
			v = repository.SelectAspects(value.Value, value.Units, selector)
		}
		out = append(out, federatedOf(repositoryID, commit, objectID, value, v))
		return nil
	})
	return out, err
}

func (s *Serving) ReadAddress(address kernel.Address) ([]FederatedValue, error) {
	out := []FederatedValue{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
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

func (s *Serving) Resolve(objectID kernel.ObjectID) ([]repository.Resolution, error) {
	out := []repository.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		resolution, err := repo.Resolve(objectID, commit)
		if err != nil {
			return err
		}
		if resolution.Status == repository.StatusUnresolved {
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
func (s *Serving) ResolveAddress(address kernel.Address) ([]repository.Resolution, error) {
	out := []repository.Resolution{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		resolution, err := repo.ResolveAddress(address, commit)
		if err != nil {
			return err
		}
		if resolution.Status == repository.StatusUnresolved {
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
func (s *Serving) ResolveBinding(address kernel.Address) ([]ResolvedBinding, error) {
	out := []ResolvedBinding{}
	err := s.eachRepository(func(_ kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		resolution, err := repo.ResolveAddress(address, commit)
		if err != nil {
			return err
		}
		if resolution.Status != repository.StatusResolved {
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
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
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

func (s *Serving) GetProvenance(objectID kernel.ObjectID) ([]repository.ProvenanceTrace, error) {
	out := []repository.ProvenanceTrace{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
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

func (s *Serving) Log(objectID kernel.ObjectID, limit int) ([]ObjectLog, error) {
	out := []ObjectLog{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
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

func (s *Serving) DescribeSchema(objectID kernel.ObjectID) ([]SchemaReport, error) {
	out := []SchemaReport{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo repository.Repository) error {
		report, err := DescribeRepoSchema(repo, commit, objectID)
		if err != nil {
			return err
		}
		out = append(out, report)
		return nil
	})
	return out, err
}

func (s *Serving) eachRepository(fn func(kernel.RepositoryID, kernel.CommitID, repository.Repository) error) error {
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
