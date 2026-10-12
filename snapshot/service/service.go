// Package service serves the Snapshot Store's file Dataset and access APIs.
// It borrows only file-store ports; KC application state is not a backend.
package service

import (
	"context"
	"net/http"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

type CatalogProvider interface {
	CatalogIDs() []string
	OpenCatalog(string) (*dataset.Catalog, error)
}

type Sources interface {
	Get(kernel.RepositoryID) (snapshot.Store, bool)
}

type Authorization struct {
	Caller           identity.HTTPIdentity
	Query            access.Query
	Definition       *dataset.KnowledgeSet
	Pin              *dataset.ResolvedKnowledgeSet
	CatalogDiscovery bool
}

type FileReadEvidence struct {
	Catalog    string
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
	Catalogs  CatalogProvider
	Sources   Sources
	Access    *access.Service
	Authorize func(context.Context, Authorization) error
	// DescribeRepository adds optional application-owned inventory metadata for
	// an already selected registered source. It cannot change that source ID.
	DescribeRepository func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) (map[string]any, error)
	BeforePublish      func(context.Context, identity.HTTPIdentity, *dataset.Catalog, dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error)
	PreparePublication func(context.Context, identity.HTTPIdentity, dataset.KnowledgeSet) error
	// EnsureDataset is optional legacy recipe adoption. It must use the same
	// Catalog authority; an independent file service needs no application recipe.
	EnsureDataset func(context.Context, identity.HTTPIdentity, *dataset.Catalog, string) error
	RecordRead    func(context.Context, identity.HTTPIdentity, FileReadEvidence) error
	RequestURL    string
	OpenView      func(context.Context, identity.HTTPIdentity, FileCoordinate, *dataset.Catalog, dataset.KnowledgeSet, dataset.ResolvedKnowledgeSet) (*FileView, error)
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

type Handler struct{ config Config }

func NewHandler(config Config) *Handler { return &Handler{config: config} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(w, r)
}

func (r Resources) authorize(ctx context.Context, request Authorization) error {
	if request.Caller.Principal == "" {
		return kernel.Fail(kernel.ErrUnauthenticated, "a verified caller is required")
	}
	if r.Authorize != nil {
		return r.Authorize(ctx, request)
	}
	if request.Query.Action == "identity.read" {
		return nil
	}
	if r.Access == nil || r.Access.Store == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "grant store is required")
	}
	rules, err := r.Access.Store.ReadRules(ctx)
	if err != nil {
		return err
	}
	query := request.Query
	query.Principal = request.Caller.Principal
	if request.Definition != nil {
		query.Dataset = ""
	}
	if _, ok := access.Match(rules, query); ok {
		return nil
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", query.Principal, query.Action)
}
