package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

// SetDatasetObservationPublication connects the actual Dataset authority to
// observation acceptance and lookup. begin fixes the actual authority before
// external observations, returning a synchronous acceptance closure. That closure
// invokes preparation, admits its exact ready resource and commits the original
// authority CAS; all exits release its local ticket handle. Evidence
// returns current first, followed by still-eligible original continuation views.
func (idx *Index) SetDatasetObservationPublication(begin func(context.Context, string, int) (func(func() (ports.PreparedResource, error)) error, error), evidence func(context.Context, string, int) ([]ports.ObservationReference, error)) {
	idx.observationPrepare, idx.observationEvidence = begin, evidence
}

func (idx *Index) bindDatasetObservation(ctx context.Context, owner ports.Engine, setID string, revision int, request retrieval.SearchRequest) (ports.Engine, error) {
	evidence, err := idx.observationEvidence(ctx, setID, revision)
	if err != nil {
		return nil, err
	}
	if len(evidence) == 0 {
		code := kernel.ErrCapabilityUnsatisfied
		if request.Continuation != "" {
			code = kernel.ErrPreconditionFailed
		}
		return nil, kernel.Fail(code, "Dataset observations have no accepted reference")
	}
	if request.Continuation == "" {
		binder, ok := owner.(ports.PublicationBinder)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider cannot bind accepted observations")
		}
		return binder.BindPublication(ctx, evidence[0].Evidence)
	}
	cursor, err := retrieval.DecodeContinuation(request.Continuation)
	if err != nil || cursor.Scope != "repository" || cursor.Position == "" {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid accepted observation continuation")
	}
	binder, ok := owner.(ports.PublicationContinuationBinder)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider cannot resume accepted observations")
	}
	for _, proof := range evidence {
		var bound ports.Engine
		var err error
		if proof.ContinuationUntil.IsZero() {
			bound, err = binder.BindPublicationContinuation(ctx, proof.Evidence, cursor.Position)
		} else {
			bounded, ok := owner.(ports.PublicationContinuationDeadlineBinder)
			if !ok {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider cannot enforce observation continuation cutoff")
			}
			bound, err = bounded.BindPublicationContinuationUntil(ctx, proof.Evidence, cursor.Position, proof.ContinuationUntil)
		}
		if err == nil {
			return bound, nil
		}
		// Only a different immutable reference may be tried. IO, cancellation and
		// unknown authority are never hidden by another generation's availability.
		if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			return nil, err
		}
	}
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "observation continuation is outside accepted references")
}
