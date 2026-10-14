// Package snapshot defines layer ⓪: path/blob/tree/commit/ref/CAS authority.
//
// It intentionally has no object identity, Aspect, Schema, Binding, or
// retrieval concepts. Upper layers may discover optional capabilities by
// asserting their own interfaces against a Store value.
package snapshot

import (
	"kc/kernel"
	"kc/snapshot/internal/storecontract"
)

// Store is the layer ⓪ authority contract. It exposes version coordinates and
// ref mutation only; object PUT/REMOVE is compiled by layer ② before it
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

// StoreOrigin is the optional authority-instance identity: one stable,
// non-secret coordinate of the concrete deployment that backs the store (for
// example the lakeFS endpoint origin). Two deployments of the same logical
// repository id must stay distinguishable, so upper layers that cache or name
// per-repository state assert this capability and fold the origin into their
// identity.
type StoreOrigin interface {
	Origin() string
}

// TreeReader is immutable literal path/blob access. It is intentionally
// separate from TreeStore so a native Object authority can support VFS
// reads without exposing a raw path mutation bypass.
type TreeReader interface {
	ReadFile(path string, commit kernel.CommitID) ([]byte, error)
	ListFiles(commit kernel.CommitID) ([]string, error)
}

// TreeStore adds literal path mutation for explicitly file-backed authorities.
type TreeStore interface {
	TreeReader
	ApplyTreeCommit(cs TreeChangeSet) (kernel.CommitID, error)
}

// DirectoryReader is the optional lazy tree capability used by host mounts
// and the Workspace File Gateway. It lists only one directory per call;
// callers must never emulate it with a full ListFiles scan.
type DirectoryReader interface {
	ReadDirectory(DirectoryRequest) (DirectoryPage, error)
}

// Directory request and result values contain only immutable tree coordinates.
type DirectoryRequest = storecontract.DirectoryRequest
type DirectoryEntry = storecontract.DirectoryEntry
type DirectoryPage = storecontract.DirectoryPage

// HistoryStore is an optional layer ⓪ capability. It returns first-parent
// reachable commits newest first, starting at the supplied immutable commit.
// The adapter does not interpret paths or file contents.
type HistoryStore interface {
	CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error)
}

// CommitMetadataReader exposes immutable commit provenance independently of
// file contents, host Git checkouts, and object interpretation.
type CommitMetadataReader interface {
	ReadCommitMetadata(kernel.CommitID) (CommitMetadata, error)
}

type CommitMetadata = storecontract.CommitMetadata

// ChangeStore is an optional layer ⓪ acceleration capability. Paths are
// literal tree paths changed between two immutable commits.
type ChangeStore interface {
	ChangedPaths(from, to kernel.CommitID) ([]string, error)
}

// BulkTreeIngester is an optional layer ⓪ acceleration capability: one
// ApplyTreeCommit-equivalent apply through the medium's bulk data plane
// (one round trip per object instead of the presigned staging dance). CAS,
// commit identity and resulting content are identical to
// TreeStore.ApplyTreeCommit; only the object transport differs. Mediums
// without the capability stay without it — callers must fail closed, never
// silently fall back.
type BulkTreeIngester interface {
	ApplyTreeCommitBulk(cs TreeChangeSet) (kernel.CommitID, error)
}

func BulkTreeIngesterOf(store Store) (BulkTreeIngester, bool) {
	bulk, ok := store.(BulkTreeIngester)
	return bulk, ok
}

func TreeStoreOf(store Store) (TreeStore, bool) {
	tree, ok := store.(TreeStore)
	return tree, ok
}

func TreeReaderOf(store Store) (TreeReader, bool) {
	tree, ok := store.(TreeReader)
	return tree, ok
}

func DirectoryReaderOf(store Store) (DirectoryReader, bool) {
	directory, ok := store.(DirectoryReader)
	return directory, ok
}

func HistoryStoreOf(store Store) (HistoryStore, bool) {
	history, ok := store.(HistoryStore)
	return history, ok
}

func ChangeStoreOf(store Store) (ChangeStore, bool) {
	changes, ok := store.(ChangeStore)
	return changes, ok
}

// Tree changes are literal paths and bytes; private backends share these DTOs.
type TreeChange = storecontract.TreeChange
type TreeChangeSet = storecontract.TreeChangeSet

const DefaultRef = storecontract.DefaultRef

func RefOrDefault(ref string) string { return storecontract.RefOrDefault(ref) }

// Registry contains opened layer ⓪ members. Registration never asks whether a
// commit contains Knowledge Catalog files.
type Registry interface {
	SetWrap(func(Store) Store)
	Add(Store) error
	Get(kernel.RepositoryID) (Store, bool)
	Delete(kernel.RepositoryID)
	Require(kernel.RepositoryID, kernel.ErrorCode) (Store, error)
	IDs() []kernel.RepositoryID
	Close() error
	OnAdvanced(func(Advanced))
	NotifyAdvanced(Advanced)
}
