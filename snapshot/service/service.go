// Package service serves the Snapshot Store's file Dataset and access APIs.
// It borrows only file-store ports; KC application state is not a backend.
package service

import (
	"context"
	"net/http"

	"kc/kernel"
	"kc/snapshot/access"
)

type handler struct{ config Config }

func NewHandler(config Config) Handler  { return newHandler(config) }
func newHandler(config Config) *handler { return &handler{config: config} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if r.Access == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "grant store is required")
	}
	rules, err := r.Access.Rules(ctx)
	if err != nil {
		return err
	}
	query := request.Query
	query.Principal = request.Caller.Principal
	if request.Definition != nil {
		query.Dataset = ""
	}
	match := access.Match
	if query.Action == access.RepositoryDatasetPublish {
		match = access.MatchSourcePublication
	} else if request.NativeRepository {
		match = access.MatchRepository
	}
	if _, ok := match(rules, query); ok {
		return nil
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", query.Principal, query.Action)
}
