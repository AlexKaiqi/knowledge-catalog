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
}

// BatchReadStore is the optional knowledge-layer hydration capability used by
// retrieval executors. Missing object IDs are omitted; every returned value is
// assembled from the requested immutable commit.
type BatchReadStore interface {
	ReadMany(objectIDs []ObjectID, commitID kernel.CommitID) (map[ObjectID]KnowledgeValue, error)
}

// ObjectIDPage is identity-only maintenance input. It contains no object body,
// relation endpoint, predicate, or filterable field.
type ObjectIDPage struct {
	ObjectIDs    []ObjectID
	Continuation string
	Exhausted    bool
}

// SnapshotObjectPager is the optional bounded identity enumeration seam used
// by explicit projection rebuild and export. Consumer READ/SEARCH/RELATIONS
// must never call it; maintenance combines it with same-commit ReadMany.
type SnapshotObjectPager interface {
	ObjectIDsPage(commitID kernel.CommitID, limit int, continuation string) (ObjectIDPage, error)
}

// UnitLocator maps one knowledge identity to its bounded unit paths at an
// immutable commit. It is the exact-read seam for tree-backed authorities;
// implementations must not satisfy a point lookup by listing the whole tree.
type UnitLocator interface {
	ObjectUnitPaths(objectID ObjectID, commitID kernel.CommitID) ([]string, error)
}

// ChangeStore is the optional native layer ② write capability. Providers that
// implement it apply a bounded ChangeSet without reconstructing a Snapshot
// tree. File-backed repositories continue through the tree codec.
type ChangeStore interface {
	ApplyKnowledgeChange(commandID string, changeSet ChangeSet) (kernel.CommitID, error)
}

// SchemaStore is an optional native locator for the small schema/* namespace.
// Returned IDs are hydrated through the same immutable Repository basis.
type SchemaStore interface {
	SchemaObjectIDs(commitID kernel.CommitID) ([]ObjectID, error)
}

// BindingLocator is an optional provider-neutral locator for schemas used by
// State/Stream Binding declarations. Consumer planning may inspect this
// bounded namespace; it must never discover bindings by scanning a Snapshot.
type BindingLocator interface {
	BindingSchemaObjectIDs(commitID kernel.CommitID) ([]ObjectID, error)
}

// SchemaReferrerLocator is an optional bounded reverse index over schema_ref.
// Publishing a Domain Schema must prove the change is safe for the instances
// that already reference it, so the maintenance path needs referrers without
// walking the Snapshot. Implementations must answer from a versioned index at
// the same immutable basis; a full scan is not an acceptable fallback.
type SchemaReferrerLocator interface {
	SchemaReferrerAddresses(schema ObjectID, commitID kernel.CommitID) ([]Address, error)
}

// Repository is the read-only layer ② view created by knowledge/reader over a
// Snapshot TreeStore. Concrete Snapshot adapters never implement it.
type Repository interface {
	snapshot.Store
	ReadStore
}

// NativeRepository marks a provider that intentionally owns the complete
// layer ② implementation. Reader must not infer this from Repository alone,
// because test/file wrappers may expose read methods while still requiring the
// shared batch/cache interpreter.
type NativeRepository interface {
	Repository
	NativeKnowledgeRepository()
}

type Surface string

const (
	SurfaceCommit   Surface = "COMMIT"
	SurfaceProposal Surface = "PROPOSAL"
)
