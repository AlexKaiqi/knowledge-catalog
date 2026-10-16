// Package service provides the independent Snapshot Store HTTP assembly.
package service

import (
	"context"
	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"net/http"
)

type Sources interface {
	Get(kernel.RepositoryID) (snapshot.Store, bool)
}

type Authorization struct {
	// NativeRepository selects raw Repo scope; Dataset/Object/Aspect grants do not open it.
	NativeRepository   bool
	Caller             identity.HTTPIdentity
	Query              access.Query
	Definition         *dataset.Dataset
	Pin                *dataset.ResolvedDataset
	InventoryDiscovery bool
}

type FileReadEvidence struct {
	Pin        dataset.ResolvedDataset
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	Path       string
	File       string
	MountPath  string
	Bytes      int
	Err        error
}

// Resources is one borrowed file-service lease. An embedding application may
// add its own policy, publication preparation and evidence, without replacing
// Dataset publication or grant authority with an application executor.
type Resources struct {
	// Actual unique bindings on Config.NativeOrigin, never guessed coordinates.
	NativeRepositories map[string]kernel.RepositoryID
	// Creation reuses the host durable allocation and registration lifecycle.
	CreateNativeRepository func(context.Context, identity.HTTPIdentity, string) (kernel.RepositoryID, string, error)
	Datasets               dataset.Service
	Sources                Sources
	Access                 access.Service
	Authorize              func(context.Context, Authorization) error
	// DescribeRepository adds optional application-owned inventory metadata for
	// an already selected registered source. It cannot change that source ID.
	DescribeRepository func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) (map[string]any, error)
	// DiscoveryWorkspaceID is optional host navigation metadata, not a second
	// registration space or authorization decision.
	DiscoveryWorkspaceID string
	// BeforeAttach verifies optional host-owned durable connection bindings.
	// It must not register a source; membership is accepted by Datasets alone.
	BeforeAttach  func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) error
	BeforePublish func(context.Context, identity.HTTPIdentity, dataset.Service, dataset.Dataset) (dataset.Dataset, map[string]any, error)
	// PreparePublication readies derived capabilities for the frozen candidate
	// and returns it with any reference dependencies it froze. It may not
	// change the candidate's identity or sources.
	PreparePublication func(context.Context, identity.HTTPIdentity, dataset.Dataset) (dataset.Dataset, error)
	// EnsureDataset is optional host coordination before named consumption.
	// KC only confirms an accepted definition. A host that adopts a recipe must
	// enforce target management and source publication permission before that
	// one-time acceptance; ordinary consumption does not require source grants.
	EnsureDataset func(context.Context, identity.HTTPIdentity, dataset.Service, string) error
	// RecordRead persists one final file:read outcome before delivery. A failed
	// receipt prevents successful bytes; it does not replace a source error.
	RecordRead func(context.Context, identity.HTTPIdentity, FileReadEvidence) error
	RequestURL string
	// OpenView must use the supplied per-request source evidence callback.
	OpenView func(context.Context, identity.HTTPIdentity, FileCoordinate, dataset.Service, dataset.Dataset, dataset.ResolvedDataset, func(FileReadEvidence) error) (FileView, error)
	around   func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error)
}

type IdentityOptions struct {
	// Mode labels an unconfigured service in error messages.
	Mode string
	// Authenticator is the selected pairing; nil fails closed.
	Authenticator identity.HTTPAuthenticator
	// BrowserLogin is optional discovery metadata owned by an external login
	// provider; this file service neither implements nor interprets that flow.
	BrowserLogin any
	// ProviderURL links to the existing identity authority, never the storage account.
	ProviderURL string
	// LogoutURL terminates a browser session at its existing identity gateway.
	LogoutURL string
}

type Config struct {
	// Trusted native instance. NativeCredential supplies server-private storage
	// proof after caller authentication; it never becomes a caller identity.
	NativeOrigin          string
	NativeCredential      func(context.Context) (string, error)
	NativeManagedCreation bool
	Identity              IdentityOptions
	// Acquire returns narrow resources plus a release function. It may hold a
	// caller's lease/lock but must never expose Home or Invocation to this module.
	Acquire func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error)
	// Observe is an optional application aspect. The module passes the actual
	// result/error; an observer cannot replace authorization or domain results.
	Observe func(context.Context, identity.HTTPIdentity, string) (context.Context, func(any, error))
	// Around preserves an embedding application's before/after hooks. A
	// rejected pre-hook may stop run; accepted work executes at most once and
	// retains its actual result. No store authority is delegated.
	Around func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error)
}

// Handler serves and registers the closed file/access route set.
type Handler interface {
	http.Handler
	Register(*http.ServeMux)
}

// FileView reads an already authorized immutable file scope.
type FileView interface {
	FixedPin() dataset.ResolvedDataset
	VisibleMounts() []dataset.VirtualMount
	List(request FileDirectoryRequest) (FileDirectoryResponse, error)
	Read(request FileReadRequest) (FileReadResponse, error)
}
