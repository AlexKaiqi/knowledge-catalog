// Package object defines layer ②: versioned identity, Aspect values,
// provenance, Schema and stable Binding declarations over a Snapshot.
package object

import (
	"context"

	"kc/kernel"
)

// ReadStore interprets object units at a fixed Snapshot commit.
type ReadStore interface {
	Resolve(objectID ObjectID, commitID kernel.CommitID) (Resolution, error)
	Read(objectID ObjectID, commitID kernel.CommitID) (ObjectValue, error)
	ResolveAddress(address Address, commitID kernel.CommitID) (Resolution, error)
	ReadAddress(address Address, commitID kernel.CommitID) (ObjectValue, error)
	GetProvenance(objectID ObjectID, commitID kernel.CommitID) (ProvenanceTrace, error)
	Log(objectID ObjectID, commitID kernel.CommitID, query ObjectLogQuery) ([]ObjectRevision, error)
	Diff(objectID ObjectID, from, to kernel.CommitID) (ObjectDiff, error)
}

// ContextBound is the optional capability of a Repository whose reads reach a
// remote authority. WithContext returns a view of the same Repository whose
// reads are canceled with ctx; a Repository that never leaves the process has
// no use for it and does not implement it.
type ContextBound interface {
	WithContext(ctx context.Context) Repository
}

// BindContext binds repo's reads to ctx when repo can be bound and otherwise
// returns repo unchanged.
func BindContext(ctx context.Context, repo Repository) Repository {
	if bound, ok := repo.(ContextBound); ok && ctx != nil {
		return bound.WithContext(ctx)
	}
	return repo
}

// BatchReadStore is the optional object-layer hydration capability used by
// retrieval executors. Missing object IDs are omitted; every returned value is
// assembled from the requested immutable commit.
type BatchReadStore interface {
	ReadMany(objectIDs []ObjectID, commitID kernel.CommitID) (map[ObjectID]ObjectValue, error)
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

// UnitLocator maps one object identity to its bounded unit paths at an
// immutable commit. It is the exact-read seam for tree-backed authorities;
// implementations must not satisfy a point lookup by listing the whole tree.
type UnitLocator interface {
	ObjectUnitPaths(objectID ObjectID, commitID kernel.CommitID) ([]string, error)
}

// ChangeStore is the optional native layer ② write capability. Providers that
// implement it apply a bounded ChangeSet without reconstructing a Snapshot
// tree. File-backed repositories continue through the tree codec.
type ChangeStore interface {
	ApplyObjectChange(commandID string, changeSet ChangeSet) (kernel.CommitID, error)
}

// SchemaStore is an optional native locator for the small schema/* namespace.
// Returned IDs are hydrated through the same immutable Repository basis.
type SchemaStore interface {
	SchemaObjectIDs(commitID kernel.CommitID) ([]ObjectID, error)
}

// SchemaAdoptionLocator returns only the exact Schema references adopted by
// an object at the fixed basis, including their explicit historical commits.
// It reads derived declaration metadata, never object or sibling bodies.
type SchemaAdoptionLocator interface {
	AdoptedSchemaRefs(objectID ObjectID, commitID kernel.CommitID) ([]string, error)
}

// BindingLocator is an optional provider-neutral locator for schemas used by
// State/Stream Binding declarations. Consumer planning may inspect this
// bounded namespace; it must never discover bindings by scanning a Snapshot.
type BindingLocator interface {
	BindingSchemaObjectIDs(commitID kernel.CommitID) ([]ObjectID, error)
}

// BindingReference is one binding unit and the ResourceDescriptor it names,
// read from bounded binding coordinates; a Dataset publication uses it to
// freeze descriptor dependencies without scanning a Snapshot.
type BindingReference struct {
	Address       Address  `json:"address"`
	DescriptorRef ObjectID `json:"descriptorRef"`
}

// SchemaReferrerLocator is an optional bounded reverse index over schema_ref.
// Publishing a Domain Schema must prove the change is safe for the instances
// that already reference it, so the maintenance path needs referrers without
// walking the Snapshot. Implementations must answer from a versioned index at
// the same immutable basis; a full scan is not an acceptable fallback.
type SchemaReferrerLocator interface {
	SchemaReferrerAddresses(schema ObjectID, commitID kernel.CommitID) ([]Address, error)
}

// Repository is the read-only layer ② view created by object/reader over a
// Snapshot TreeStore. Concrete Snapshot adapters never implement it.
type Repository interface {
	ID() kernel.RepositoryID
	Head(string) (kernel.CommitID, error)
	GetRef(string) (kernel.CommitID, bool)
	HasCommit(kernel.CommitID) bool
	Archived() bool
	ReadStore
}

// StoreIdentified is the optional authority-instance identity of a Repository:
// a stable digest that distinguishes two deployments of the same logical
// repository id. Per-repository caches and projections must fold it into their
// identity so one deployment never serves another deployment's state. The
// digest is non-secret; credentials never enter it.
type StoreIdentified interface {
	StoreDigest() kernel.Digest
}

// NativeRepository marks a provider that intentionally owns the complete
// layer ② implementation. Reader must not infer this from Repository alone,
// because test/file wrappers may expose read methods while still requiring the
// shared batch/cache interpreter.
// Its Resolve methods and conditional writes share MaintenanceDigest and
// UnitSetMaintenanceDigest; content READ digests retain their value semantics.
type NativeRepository interface {
	Repository
	NativeObjectRepository()
}

type Surface string

const (
	SurfaceCommit Surface = "COMMIT"
)
