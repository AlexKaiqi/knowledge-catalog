package kernel

import (
	"encoding/json"
	"fmt"
	"testing"
)

type provenUnappliedError struct{ error }

func (provenUnappliedError) WriteNotApplied() bool { return true }
func (e provenUnappliedError) Unwrap() error       { return e.error }

func TestWriteOutcomeNormalizationPreservesProofWithoutMutatingSharedFault(t *testing.T) {
	original := Fail(ErrTemporaryUnavailable, "staging failed").(*IngressError)
	marked := fmt.Errorf("writer: %w", provenUnappliedError{original})
	normalized := Normalize(marked)
	if normalized.Code != original.Code || normalized.Message != original.Message || normalized.WriteOutcome != WriteOutcomeNotApplied {
		t.Fatalf("proof lost: %#v", normalized)
	}
	if original.WriteOutcome != "" || Normalize(original) != original || IsWriteNotApplied(original) {
		t.Fatalf("normalization mutated unmarked shared error: %#v", original)
	}
	raw, err := json.Marshal(FaultJSON(marked))
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Error *IngressError `json:"error"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if !IsWriteNotApplied(fmt.Errorf("client: %w", wire.Error)) {
		t.Fatalf("wire proof lost: %s", raw)
	}
}

func TestUnknownWriteOutcomesDoNotBecomeProof(t *testing.T) {
	for _, outcome := range []string{"", "UNKNOWN", "NOT_APPLIED_TO_SOMETHING_ELSE"} {
		fault := &IngressError{Code: ErrTemporaryUnavailable, Message: "not applied", WriteOutcome: outcome}
		if IsWriteNotApplied(fault) || Normalize(fault) != fault {
			t.Fatalf("inferred proof from %#v", fault)
		}
	}
	raw, err := json.Marshal(FaultJSON(Fail(ErrTemporaryUnavailable, "lost response")))
	if err != nil || string(raw) != `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"lost response"}}` {
		t.Fatalf("legacy fault shape changed: %s, %v", raw, err)
	}
}
