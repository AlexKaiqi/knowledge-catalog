package projection

import (
	"context"
	"testing"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
)

type contextOnlyMatchEngine struct {
	ports.Engine
	wanted context.Context
	called bool
}

func (e *contextOnlyMatchEngine) VerifyResidualMatchContext(ctx context.Context, _, _ string, _ retrieval.MatchMode) (bool, error) {
	e.called = ctx == e.wanted
	return false, ctx.Err()
}

func TestDatasetMatchRecheckRetainsRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &contextOnlyMatchEngine{wanted: ctx}
	verifier := matchVerifierForContext(ctx, engine)
	if verifier == nil {
		t.Fatal("context-aware provider cannot recheck hydrated candidates")
	}
	if matched, err := verifier.VerifyResidualMatch("healthy", "healthy", retrieval.MatchAllTerms); matched || err != context.Canceled || !engine.called {
		t.Fatalf("MATCH recheck lost cancellation: matched=%v error=%v called=%v", matched, err, engine.called)
	}
}
