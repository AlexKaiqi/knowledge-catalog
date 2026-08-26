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

// BatchReadStore is the optional knowledge-layer hydration capability used by
// retrieval executors. Missing object IDs are omitted; every returned value is
// assembled from the requested immutable commit.
type BatchReadStore interface {
	ReadMany(objectIDs []ObjectID, commitID kernel.CommitID) (map[ObjectID]KnowledgeValue, error)
}

// Repository is the read-only layer ② view created by knowledge/reader over a
// Snapshot TreeStore. Concrete Snapshot adapters never implement it.
type Repository interface {
	snapshot.Store
	ReadStore
}

type Surface string

const (
	SurfaceCommit   Surface = "COMMIT"
	SurfaceProposal Surface = "PROPOSAL"
)
