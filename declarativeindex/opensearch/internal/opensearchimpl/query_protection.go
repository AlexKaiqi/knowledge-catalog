package opensearchimpl

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"sync"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// A query lease has a fixed deadline. It is neither an authority admission nor
// a continuation lease, and may never be confirmed into an indefinite root.
type queryProtection struct {
	continuation *resourceClaim
	claim        resourceClaim
	session      *rebuildSession
	ctx          context.Context
	cancel       context.CancelFunc
	until        int64
	admittedAt   int64
	release      func() error
	once         sync.Once
	err          error
}

func (p *preparedResource) protectQuery(ctx context.Context, evidence []byte) (*queryProtection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	until := time.Now().Add(ports.DefaultSearchTimeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(until) {
		until = deadline
	}
	if !until.After(time.Now()) {
		return nil, context.DeadlineExceeded
	}
	id, err := newResourceID()
	if err != nil {
		return nil, err
	}
	phase := claimQuery
	if p.rootQuery != nil {
		phase = claimRootQuery
	}
	claim := resourceClaim{ID: id, Phase: phase, Until: until.UnixMilli(), Evidence: bytes.Clone(evidence)}
	session := *p.session
	session.resourceContext = ctx
	if p.observation != nil && p.observation.until > 0 {
		admissionContext, done := context.WithDeadline(ctx, time.UnixMilli(p.observation.until))
		defer done()
		session.resourceContext = admissionContext
	}
	if err := session.changeResourceRecord(func(record *controlDoc) error {
		resource := record.Resource
		if p.observation != nil && p.observation.until > 0 && p.observation.until <= time.Now().UnixMilli() {
			return expiredObservation()
		}

		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "query resource is no longer available")
		}
		held := slices.ContainsFunc(resource.Claims, func(accepted resourceClaim) bool {
			if p.queryAnchor != nil {
				anchor := *p.queryAnchor
				return accepted.ID == anchor.ID && accepted.Phase == anchor.Phase && accepted.Until == anchor.Until && accepted.Until > time.Now().UnixMilli() && bytes.Equal(accepted.Evidence, anchor.Evidence)
			}
			return accepted.ID == p.id && (accepted.Phase == claimAdmitted || accepted.Phase == claimAccepted) && bytes.Equal(accepted.Evidence, evidence)
		})
		if p.observation != nil && p.queryAnchor == nil {
			held = record.Cause == "observation" && reflect.DeepEqual(readyResourceMeta(*record), readyResourceMeta(p.observation.record))
		}
		if p.rootQuery != nil && p.queryAnchor == nil {
			held = resource.QueryLeases == 1 && reflect.DeepEqual(readyResourceMeta(*record), readyResourceMeta(*p.rootQuery))
		}
		if !held {
			return kernel.Fail(kernel.ErrPreconditionFailed, "publication reference changed before query admission")
		}
		resource.Claims = append(resource.Claims, claim)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := session.resourceCtx().Err(); err != nil {
		return nil, err
	}
	if p.observation != nil && p.observation.until > 0 && p.observation.until <= time.Now().UnixMilli() {
		return nil, expiredObservation()
	}
	// A lost response above may leave a bounded query lease. It is safe to let
	// that exact deadline expire, but never to reinterpret it as publication.
	queryCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(claim.Until))
	q := &queryProtection{claim: claim, session: &session, ctx: queryCtx, cancel: cancel, until: claim.Until, admittedAt: time.Now().UnixMilli()}
	q.continuation = p.queryAnchor
	q.release = func() error {
		cleanup, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		release := session
		release.resourceContext = cleanup
		return release.changeResource(func(resource *generationResource) error {
			for i, actual := range resource.Claims {
				if actual.ID != claim.ID {
					continue
				}
				if actual.Phase != claim.Phase || actual.Until != claim.Until || !bytes.Equal(actual.Evidence, claim.Evidence) {
					return kernel.Fail(kernel.ErrPreconditionFailed, "query protection identity changed")
				}
				resource.Claims = slices.Delete(resource.Claims, i, i+1)
				return nil
			}
			return nil
		})
	}
	return q, nil
}

func (q *queryProtection) close() error {
	q.once.Do(func() { q.cancel(); q.err = q.release() })
	return q.err
}

func (q *queryProtection) context(ctx context.Context) (context.Context, func(), error) {
	if err := q.status(); err != nil {
		return nil, nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bounded, cancel := context.WithDeadline(ctx, time.UnixMilli(q.until))
	stop := context.AfterFunc(q.ctx, cancel)
	return bounded, func() { stop(); cancel() }, nil
}

// Deadline discharge is local to query leases; it never discharges an
// uncertain publication. CAS rechecks the current resource and each deadline.
func (s *rebuildSession) releaseExpiredQueries() ([]string, error) {
	var released []string
	err := s.changeResource(func(resource *generationResource) error {
		released = nil
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "query resource changed during lease recovery")
		}
		now := time.Now().UnixMilli()
		resource.Claims = slices.DeleteFunc(resource.Claims, func(claim resourceClaim) bool {
			expired := (claim.Phase == claimQuery || claim.Phase == claimContinuation || claim.Phase == claimRootQuery || claim.Phase == claimRootContinuation) && claim.Until <= now
			if expired {
				released = append(released, s.physicalIndex+"/"+claim.ID)
			}
			return expired
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return released, nil
}

func (q *queryProtection) status() error {
	if time.Now().UnixMilli() >= q.until {
		return kernel.Fail(kernel.ErrPreconditionFailed, "query resource protection expired; restart the query")
	}
	return q.ctx.Err()
}
