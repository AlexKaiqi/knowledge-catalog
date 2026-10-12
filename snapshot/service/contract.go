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
	Caller             identity.HTTPIdentity
	Query              access.Query
	Definition         *dataset.KnowledgeSet
	Pin                *dataset.ResolvedKnowledgeSet
	InventoryDiscovery bool
}

type FileReadEvidence struct {
	Pin        dataset.ResolvedKnowledgeSet
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
	Datasets  dataset.Service
	Sources   Sources
	Access    access.Service
	Authorize func(context.Context, Authorization) error
	// DescribeRepository adds optional application-owned inventory metadata for
	// an already selected registered source. It cannot change that source ID.
	DescribeRepository func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) (map[string]any, error)
	BeforePublish      func(context.Context, identity.HTTPIdentity, dataset.Service, dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error)
	PreparePublication func(context.Context, identity.HTTPIdentity, dataset.KnowledgeSet) error
	// EnsureDataset is optional legacy recipe adoption. It must use the same
	// Dataset authority; an independent file service needs no application recipe.
	EnsureDataset func(context.Context, identity.HTTPIdentity, dataset.Service, string) error
	RecordRead    func(context.Context, identity.HTTPIdentity, FileReadEvidence) error
	RequestURL    string
	OpenView      func(context.Context, identity.HTTPIdentity, FileCoordinate, dataset.Service, dataset.KnowledgeSet, dataset.ResolvedKnowledgeSet) (FileView, error)
	around        func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error)
}

type IdentityOptions struct {
	Mode          string
	Local         bool
	Authenticator identity.HTTPAuthenticator
	// BrowserLogin is optional discovery metadata owned by an external login
	// provider; this file service neither implements nor interprets that flow.
	BrowserLogin any
}

type Config struct {
	Identity IdentityOptions
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
	FixedPin() dataset.ResolvedKnowledgeSet
	VisibleMounts() []dataset.VirtualMount
	List(request FileDirectoryRequest) (FileDirectoryResponse, error)
	Read(request FileReadRequest) (FileReadResponse, error)
}
