package liveaccess

import (
	"context"
	"time"

	"kc/kernel"
)

// ReadLimits bounds the external lookups of one read. Lookup is each unit's
// own limit; Read is the budget all units of the request share, counted from
// when the request entered the Server. Zero leaves that limit unset.
type ReadLimits struct {
	Lookup time.Duration
	Read   time.Duration
}

type readLimitsKey struct{}

type readBudget struct {
	lookup   time.Duration
	deadline time.Time
}

// ContextWithReadLimits starts one request's shared budget now. The budget is
// carried as a value rather than a context deadline: only external lookups
// share it, so Snapshot reads, evidence and the response are never cut off.
func ContextWithReadLimits(ctx context.Context, limits ReadLimits) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits.Lookup <= 0 && limits.Read <= 0 {
		return ctx
	}
	budget := readBudget{lookup: limits.Lookup}
	if limits.Read > 0 {
		budget.deadline = time.Now().Add(limits.Read)
	}
	return context.WithValue(ctx, readLimitsKey{}, budget)
}

// WithoutReadBudget keeps each unit's own limit but releases the shared budget
// of a consumer read. Index preparation observes under the service identity;
// it is maintenance, not a read, so building a projection first must not leave
// its later observations with an exhausted read budget.
func WithoutReadBudget(ctx context.Context) context.Context {
	budget, ok := ctx.Value(readLimitsKey{}).(readBudget)
	if !ok || budget.deadline.IsZero() {
		return ctx
	}
	budget.deadline = time.Time{}
	return context.WithValue(ctx, readLimitsKey{}, budget)
}

// UnitLookupContext bounds one unit by the smaller of its own limit and the
// request's remaining time. An exhausted request budget fails the unit before
// any call, so later units report a timeout instead of extending the request.
func UnitLookupContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	budget, _ := ctx.Value(readLimitsKey{}).(readBudget)
	now := time.Now()
	if !budget.deadline.IsZero() && !now.Before(budget.deadline) {
		return nil, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "external lookup timed out: the read's time limit was used up before this unit started")
	}
	deadline := budget.deadline
	if budget.lookup > 0 {
		if own := now.Add(budget.lookup); deadline.IsZero() || own.Before(deadline) {
			deadline = own
		}
	}
	if deadline.IsZero() {
		return ctx, func() {}, nil
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	return bounded, cancel, nil
}

// LookupTimedOut reports whether a unit failed because its bound expired while
// the request itself is still live; the caller replaces the transport error.
func LookupTimedOut(request, unit context.Context) bool {
	return request.Err() == nil && unit.Err() == context.DeadlineExceeded
}
