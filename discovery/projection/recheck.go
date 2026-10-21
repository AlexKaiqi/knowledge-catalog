package projection

import (
	"context"
	retrieval "kc/discovery"
	objectmodel "kc/object"
)

type candidateRecheckKey struct{}

// CandidateRecheck re-reads one candidate as the authorized consumer. A unit
// whose external value could not be adopted is returned beside the value, so
// the projection can explain it instead of matching or dropping the candidate.
type CandidateRecheck func(context.Context, retrieval.ObjectHit) (retrieval.ObjectHit, []objectmodel.UnitReadFailure, error)

// WithCandidateRecheck lets the authorized consumer re-read an observation
// candidate before it occupies a result slot. The index retains its own basis.
func WithCandidateRecheck(ctx context.Context, recheck CandidateRecheck) context.Context {
	return context.WithValue(ctx, candidateRecheckKey{}, recheck)
}

// RecheckCandidate reports whether a recheck ran, with the reread hit and the
// units whose reread failed.
func RecheckCandidate(ctx context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, []objectmodel.UnitReadFailure, bool, error) {
	if recheck, ok := ctx.Value(candidateRecheckKey{}).(CandidateRecheck); ok {
		value, failures, err := recheck(ctx, hit)
		return value, failures, true, err
	}
	return hit, nil, false, nil
}
