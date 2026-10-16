package projection

import (
	"context"
	retrieval "kc/discovery"
)

type candidateRecheckKey struct{}

// WithCandidateRecheck lets the authorized consumer re-read an observation
// candidate before it occupies a result slot. The index retains its own basis.
func WithCandidateRecheck(ctx context.Context, recheck func(context.Context, retrieval.ObjectHit) (retrieval.ObjectHit, error)) context.Context {
	return context.WithValue(ctx, candidateRecheckKey{}, recheck)
}

func RecheckCandidate(ctx context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, bool, error) {
	if recheck, ok := ctx.Value(candidateRecheckKey{}).(func(context.Context, retrieval.ObjectHit) (retrieval.ObjectHit, error)); ok {
		value, err := recheck(ctx, hit)
		return value, true, err
	}
	return hit, false, nil
}
