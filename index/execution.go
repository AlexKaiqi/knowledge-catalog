package index

import (
	"context"
	"errors"
	"sync"
	"time"

	"kc/kernel"
	"kc/retrieval"
)

// SearchBudget is execution policy, not a SEARCH predicate or an index hint.
// Zero values select the defaults. A Workspace shares one budget across all
// member queries through WithSearchBudget rather than resetting it per member.
type SearchBudget struct {
	MaxCandidates int
	MaxPages      int
	Timeout       time.Duration
}

const (
	DefaultSearchCandidateBudget = 10000
	DefaultSearchPageBudget      = 100
	DefaultSearchTimeout         = 30 * time.Second
)

var errSearchBudgetDeadline = errors.New("search execution time budget exhausted")

type searchBudgetKey struct{}
type searchBudgetState struct {
	mu         sync.Mutex
	candidates int
	pages      int
	inFlight   int
	changed    chan struct{}
}

// WithSearchBudget establishes one shared, concurrency-safe execution scope.
// An existing scope is preserved so nested repository queries cannot reset it.
func WithSearchBudget(ctx context.Context, budget SearchBudget) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(searchBudgetKey{}).(*searchBudgetState); ok {
		return ctx, func() {}
	}
	if budget.MaxCandidates <= 0 {
		budget.MaxCandidates = DefaultSearchCandidateBudget
	}
	if budget.MaxPages <= 0 {
		budget.MaxPages = DefaultSearchPageBudget
	}
	if budget.Timeout <= 0 {
		budget.Timeout = DefaultSearchTimeout
	}
	state := &searchBudgetState{candidates: budget.MaxCandidates, pages: budget.MaxPages, changed: make(chan struct{})}
	ctx = context.WithValue(ctx, searchBudgetKey{}, state)
	return context.WithTimeoutCause(ctx, budget.Timeout, errSearchBudgetDeadline)
}

func searchContextStatus(ctx context.Context) (string, error) {
	if ctx.Err() == nil {
		return "", nil
	}
	if errors.Is(context.Cause(ctx), errSearchBudgetDeadline) {
		return "time", nil
	}
	return "", ctx.Err()
}

// reserveSearchPage bounds even concurrent candidate requests. Unused capacity
// is returned when a short page completes; waiting requests observe the refund.
func reserveSearchPage(ctx context.Context, requested int) (int, func(int), string, error) {
	state := ctx.Value(searchBudgetKey{}).(*searchBudgetState)
	for {
		if reason, err := searchContextStatus(ctx); reason != "" || err != nil {
			return 0, nil, reason, err
		}
		state.mu.Lock()
		if state.pages == 0 {
			state.mu.Unlock()
			return 0, nil, "pages", nil
		}
		if state.candidates > 0 {
			if requested > state.candidates {
				requested = state.candidates
			}
			state.candidates -= requested
			state.pages--
			state.inFlight++
			state.mu.Unlock()
			finish := func(actual int) {
				state.mu.Lock()
				if actual >= 0 && actual < requested {
					state.candidates += requested - actual
				}
				state.inFlight--
				close(state.changed)
				state.changed = make(chan struct{})
				state.mu.Unlock()
			}
			return requested, finish, "", nil
		}
		if state.inFlight == 0 {
			state.mu.Unlock()
			return 0, nil, "candidates", nil
		}
		changed := state.changed
		state.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
	}
}

func markSearchBudget(result *retrieval.SearchResult, reason string) {
	result.Completeness = retrieval.CompletenessPartial
	result.Stats.MarkPartial("budget")
	result.Claims = append(result.Claims, "search execution budget exhausted: "+reason)
}

// SearchBudgetExhausted distinguishes the execution deadline from caller cancellation.
func SearchBudgetExhausted(ctx context.Context) bool {
	return ctx != nil && errors.Is(context.Cause(ctx), errSearchBudgetDeadline)
}

func searchPreparationError(ctx context.Context, err error) error {
	if err != nil && SearchBudgetExhausted(ctx) {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "search execution time budget exhausted before a fixed retrieval plan was established")
	}
	return err
}
