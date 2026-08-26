// Package knowledge defines layer ②: versioned identity, Aspect values,
// provenance, Schema and stable Binding declarations over a Snapshot.
package knowledge

import (
	"kc/kernel"
	"kc/snapshot"
)

// ReadStore interprets knowledge units at a fixed Snapshot commit.
type ReadStore interface {
	Resolve(objectID ObjectID, commitID kernel.CommitID) (Resolution, error)
	Read(objectID ObjectID, commitID kernel.CommitID) (KnowledgeValue, error)
	ResolveAddress(address Address, commitID kernel.CommitID) (Resolution, error)
	ReadAddress(address Address, commitID kernel.CommitID) (KnowledgeValue, error)
	GetProvenance(objectID ObjectID, commitID kernel.CommitID) (ProvenanceTrace, error)
	Log(objectID ObjectID, commitID kernel.CommitID, limit int) ([]ObjectRevision, error)
	Diff(objectID ObjectID, from, to kernel.CommitID) (ObjectDiff, error)
	List(commitID kernel.CommitID) ([]KnowledgeValue, error)
}

// WriteStore accepts layer ② PUT/REMOVE and owns their translation to a
// layer ⓪ tree commit. Snapshot Store deliberately does not expose this method.
type WriteStore interface {
	ApplyKnowledgeCommit(ChangeSet) (kernel.CommitID, error)
}

// Repository is the optional layer ② capability of a Snapshot member.
type Repository interface {
	snapshot.Store
	ReadStore
	WriteStore
}

func Of(store snapshot.Store) (Repository, bool) {
	repo, ok := store.(Repository)
	return repo, ok
}

func Require(registry *snapshot.Registry, id kernel.RepositoryID, code kernel.ErrorCode) (Repository, error) {
	store, err := registry.Require(id, code)
	if err != nil {
		return nil, err
	}
	repo, ok := Of(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s is mounted as a plain snapshot and does not interpret knowledge files", id)
	}
	return repo, nil
}

// Lookup adapts a pure Snapshot lookup at an application seam. Catalog remains
// layer ①; the caller that assembles Reader/Index chooses to require layer ②.
func Lookup(base func(kernel.RepositoryID) (snapshot.Store, error)) func(kernel.RepositoryID) (Repository, error) {
	return func(id kernel.RepositoryID) (Repository, error) {
		store, err := base(id)
		if err != nil {
			return nil, err
		}
		repo, ok := Of(store)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s is mounted as a plain snapshot and does not interpret knowledge files", id)
		}
		return repo, nil
	}
}

type Surface string

const (
	SurfaceCommit   Surface = "COMMIT"
	SurfaceProposal Surface = "PROPOSAL"
)
