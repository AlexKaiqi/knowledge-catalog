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
	ListPage(commitID kernel.CommitID, request PageRequest) (KnowledgePage, error)
}

// PageRequest is the bounded browse input shared by knowledge providers. The
// continuation is provider-owned and is valid only for the immutable commit
// that produced it.
type PageRequest struct {
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

// KnowledgePage is one bounded object-id ordered page. Exhausted is explicit
// so callers never infer completion from a short page.
type KnowledgePage struct {
	Values       []KnowledgeValue `json:"values"`
	Continuation string           `json:"continuation,omitempty"`
	Exhausted    bool             `json:"exhausted"`
}

const (
	DefaultPageLimit = 100
	MaxPageLimit     = 1000
)

func NormalizePageLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "page limit cannot be negative")
	}
	if limit == 0 {
		return DefaultPageLimit, nil
	}
	if limit > MaxPageLimit {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "page limit cannot exceed %d", MaxPageLimit)
	}
	return limit, nil
}

// WalkPages visits a fixed commit without ever materializing the repository in
// one slice. Providers retain ownership of continuation encoding.
func WalkPages(store ReadStore, commitID kernel.CommitID, visit func(KnowledgeValue) error) error {
	request := PageRequest{Limit: MaxPageLimit}
	for {
		page, err := store.ListPage(commitID, request)
		if err != nil {
			return err
		}
		for _, value := range page.Values {
			if err := visit(value); err != nil {
				return err
			}
		}
		if page.Exhausted {
			return nil
		}
		if page.Continuation == "" || page.Continuation == request.Continuation {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "knowledge provider returned a non-advancing page")
		}
		request.Continuation = page.Continuation
	}
}

// BatchReadStore is the optional knowledge-layer hydration capability used by
// retrieval executors. Missing object IDs are omitted; every returned value is
// assembled from the requested immutable commit.
type BatchReadStore interface {
	ReadMany(objectIDs []ObjectID, commitID kernel.CommitID) (map[ObjectID]KnowledgeValue, error)
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
