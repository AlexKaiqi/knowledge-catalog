package projection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

type acceptedObservationEngine struct {
	ports.Engine
	proofs           []string
	positions        []string
	current, resumed ports.Engine
	resumeError      error
}

func (e *acceptedObservationEngine) BindPublication(_ context.Context, proof []byte) (ports.Engine, error) {
	e.proofs = append(e.proofs, string(proof))
	return e.current, nil
}
func (e *acceptedObservationEngine) BindPublicationContinuation(_ context.Context, proof []byte, position string) (ports.Engine, error) {
	e.proofs = append(e.proofs, string(proof))
	e.positions = append(e.positions, position)
	if e.resumeError != nil {
		return nil, e.resumeError
	}
	if string(proof) == "original" {
		return e.resumed, nil
	}
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "different immutable admission")
}

func TestAcceptedObservationQueriesSelectAuthorityEvidenceBeforeProviderROOT(t *testing.T) {
	idx := NewIndexEngine("", nil)
	original, current := &acceptedObservationEngine{}, &acceptedObservationEngine{}
	owner := &acceptedObservationEngine{current: current, resumed: original}
	idx.SetDatasetObservationPublication(nil, func(context.Context, string, int) ([]ports.ObservationReference, error) {
		return []ports.ObservationReference{{Evidence: []byte("accepted-current")}, {Evidence: []byte("original"), ContinuationUntil: time.Unix(2000000000, 0)}}, nil
	})
	bound, err := idx.bindDatasetObservation(context.Background(), owner, "named", 1, retrieval.SearchRequest{})
	if err != nil || bound != current || !reflect.DeepEqual(owner.proofs, []string{"accepted-current"}) {
		t.Fatalf("new query chose unaccepted resource: %#v %v", owner, err)
	}
	owner.proofs = nil
	token := retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "repository", Position: "original-provider-position"})
	bound, err = idx.bindDatasetObservation(context.Background(), owner, "named", 1, retrieval.SearchRequest{Continuation: token})
	if err != nil || bound != original || !reflect.DeepEqual(owner.proofs, []string{"accepted-current", "original"}) || !reflect.DeepEqual(owner.positions, []string{"original-provider-position", "original-provider-position"}) {
		t.Fatalf("old continuation changed evidence/position: %#v %v", owner, err)
	}
}

func TestAcceptedObservationUnknownErrorsCannotFallThroughToAnotherGeneration(t *testing.T) {
	idx := NewIndexEngine("", nil)
	problem := errors.New("authority transport unavailable")
	owner := &acceptedObservationEngine{resumeError: problem}
	idx.SetDatasetObservationPublication(nil, func(context.Context, string, int) ([]ports.ObservationReference, error) {
		return []ports.ObservationReference{{Evidence: []byte("current")}, {Evidence: []byte("original"), ContinuationUntil: time.Unix(2000000000, 0)}}, nil
	})
	token := retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "repository", Position: "position"})
	if _, err := idx.bindDatasetObservation(context.Background(), owner, "named", 1, retrieval.SearchRequest{Continuation: token}); !errors.Is(err, problem) || len(owner.proofs) != 1 {
		t.Fatalf("unknown failure fell through: %#v %v", owner, err)
	}
	idx.SetDatasetObservationPublication(nil, func(context.Context, string, int) ([]ports.ObservationReference, error) {
		return []ports.ObservationReference{{Evidence: []byte("current")}}, nil
	})
	owner.resumeError = nil
	owner.proofs = nil
	if _, err := idx.bindDatasetObservation(context.Background(), owner, "named", 1, retrieval.SearchRequest{Continuation: token}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !reflect.DeepEqual(owner.proofs, []string{"current"}) {
		t.Fatalf("expired reference substituted current query: %#v %v", owner, err)
	}
}

func (e *acceptedObservationEngine) BindPublicationContinuationUntil(ctx context.Context, proof []byte, position string, until time.Time) (ports.Engine, error) {
	if until != time.Unix(2000000000, 0) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "authority cutoff changed")
	}
	return e.BindPublicationContinuation(ctx, proof, position)
}

func TestAcceptedObservationMissingReferencePreservesFreshAndContinuationErrors(t *testing.T) {
	idx := NewIndexEngine("", nil)
	idx.SetDatasetObservationPublication(nil, func(context.Context, string, int) ([]ports.ObservationReference, error) { return nil, nil })
	for _, test := range []struct {
		request retrieval.SearchRequest
		code    kernel.ErrorCode
	}{
		{request: retrieval.SearchRequest{}, code: kernel.ErrCapabilityUnsatisfied},
		{request: retrieval.SearchRequest{Continuation: retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "repository", Position: "original"})}, code: kernel.ErrPreconditionFailed},
	} {
		owner := &acceptedObservationEngine{}
		if _, err := idx.bindDatasetObservation(context.Background(), owner, "named", 1, test.request); kernel.CodeOf(err) != test.code || len(owner.proofs) != 0 {
			t.Fatalf("missing reference used provider or changed error: %#v %v", owner, err)
		}
	}
}
