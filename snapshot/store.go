// Package snapshot defines layer ⓪: path/blob/tree/commit/ref/CAS authority.
//
// It intentionally has no knowledge identity, Aspect, Schema, Binding, or
// retrieval concepts. Upper layers may discover optional capabilities by
// asserting their own interfaces against a Store value.
package snapshot

import "kc/kernel"

// Store is the layer ⓪ authority contract. It exposes version coordinates and
// ref mutation only; knowledge PUT/REMOVE is compiled by layer ② before it
// reaches a concrete Snapshot adapter.
type Store interface {
	ID() kernel.RepositoryID

	Head(ref string) (kernel.CommitID, error)
	GetRef(ref string) (kernel.CommitID, bool)
	HasCommit(commitID kernel.CommitID) bool
	CreateRef(ref string, commitID kernel.CommitID) error
	Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error)

	Archived() bool
	Archive() error
}

// TreeStore is an optional layer ⓪ capability for literal path/blob access.
// Paths and bytes are opaque to this package.
type TreeStore interface {
	ReadFile(path string, commit kernel.CommitID) ([]byte, error)
	ListFiles(commit kernel.CommitID) ([]string, error)
	ApplyTreeCommit(cs TreeChangeSet) (kernel.CommitID, error)
}

func TreeStoreOf(store Store) (TreeStore, bool) {
	tree, ok := store.(TreeStore)
	return tree, ok
}

type TreeChange struct {
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
	Remove  bool   `json:"remove,omitempty"`
}

type TreeChangeSet struct {
	TargetRepository     kernel.RepositoryID `json:"targetRepository"`
	TargetRef            string              `json:"targetRef"`
	BaseCommit           kernel.CommitID     `json:"baseCommit"`
	ExpectedTargetCommit kernel.CommitID     `json:"expectedTargetCommit"`
	Changes              []TreeChange        `json:"changes"`
	Message              string              `json:"message,omitempty"`
	Author               string              `json:"author,omitempty"`
	RequestID            string              `json:"requestId,omitempty"`
	RuleID               string              `json:"ruleId,omitempty"`
}

const DefaultRef = "refs/heads/main"

func RefOrDefault(ref string) string {
	if ref == "" {
		return DefaultRef
	}
	return ref
}

// Registry contains opened layer ⓪ members. Registration never asks whether a
// commit contains Knowledge Catalog files.
type Registry struct {
	stores     map[kernel.RepositoryID]Store
	onAdvanced []func(Advanced)
}

func NewRegistry() *Registry {
	return &Registry{stores: map[kernel.RepositoryID]Store{}}
}

func (r *Registry) Add(store Store) error {
	if _, ok := r.stores[store.ID()]; ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "repository %s is already registered", store.ID())
	}
	r.stores[store.ID()] = store
	return nil
}

func (r *Registry) Get(id kernel.RepositoryID) (Store, bool) {
	store, ok := r.stores[id]
	return store, ok
}

func (r *Registry) Delete(id kernel.RepositoryID) { delete(r.stores, id) }

func (r *Registry) Require(id kernel.RepositoryID, code kernel.ErrorCode) (Store, error) {
	store, ok := r.stores[id]
	if ok {
		return store, nil
	}
	if code == kernel.ErrUsageInvalid {
		return nil, kernel.Fail(code, "repository %s is not mounted", id)
	}
	return nil, kernel.Fail(code, "unknown repository %s", id)
}

func (r *Registry) IDs() []kernel.RepositoryID {
	ids := make([]kernel.RepositoryID, 0, len(r.stores))
	for id := range r.stores {
		ids = append(ids, id)
	}
	return ids
}

func (r *Registry) Close() error {
	var first error
	for id, store := range r.stores {
		if closer, ok := store.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil && first == nil {
				first = err
			}
		}
		delete(r.stores, id)
	}
	return first
}
