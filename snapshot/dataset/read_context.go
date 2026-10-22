package dataset

import (
	"context"
	"kc/kernel"
	"kc/snapshot"
)

// ReadContextBound freezes an accepted, read-only view whose source and
// registry authority I/O use the supplied request context. It owns no handles.
type ReadContextBound interface {
	ReadContext(context.Context) (Service, error)
}

func (c *service) ReadContext(ctx context.Context) (Service, error) {
	if ctx == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Dataset read context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.registry == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset registry is unavailable")
	}
	bound, err := snapshotRegistry(snapshot.BindStoreContext(c.registry.authority, ctx), c.registry.ref)
	if err != nil {
		return nil, err
	}
	bound.accept(c.accepted().head)
	as, requestID, ruleID, observer := c.requestContext()
	view := c.ReadView(observer).(*service)
	view.readContext = ctx
	view.registry = bound
	if c.store != nil {
		view.store = &contextRegistry{Registry: c.store, ctx: ctx}
	}
	view.as, view.requestID, view.ruleID = as, requestID, ruleID
	return view, nil
}

func (c *service) finishSourceRead(err *error) {
	if contextErr := c.sourceReadError(); contextErr != nil {
		*err = contextErr
	}
}

func (c *service) sourceReadError() error {
	if c.readContext != nil {
		return c.readContext.Err()
	}
	return nil
}

type contextRegistry struct {
	snapshot.Registry
	ctx context.Context
}

func (s *contextRegistry) Get(id kernel.RepositoryID) (snapshot.Store, bool) {
	store, ok := s.Registry.Get(id)
	if !ok {
		return nil, false
	}
	return snapshot.BindStoreContext(store, s.ctx), true
}
func (s *contextRegistry) Require(id kernel.RepositoryID, code kernel.ErrorCode) (snapshot.Store, error) {
	store, err := s.Registry.Require(id, code)
	if err != nil {
		return nil, err
	}
	return snapshot.BindStoreContext(store, s.ctx), nil
}
