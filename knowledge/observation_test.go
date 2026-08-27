package knowledge_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestValidateObservationBasis(t *testing.T) {
	valid := knowledge.ObservationBasis{
		BindingGeneration: "orders-health-v3",
		Consistency:       knowledge.ObservationRepeatable,
		SourceRevision:    "42",
		ObservedAt:        "2026-08-27T08:30:00.123Z",
	}
	if err := knowledge.ValidateObservationBasis(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []knowledge.ObservationBasis{
		{Consistency: knowledge.ObservationRepeatable, ObservedAt: "2026-08-27T08:30:00Z"},
		{BindingGeneration: "g1", Consistency: "eventual", ObservedAt: "2026-08-27T08:30:00Z"},
		{BindingGeneration: "g1", Consistency: knowledge.ObservationLatestOnly},
		{BindingGeneration: "g1", Consistency: knowledge.ObservationBounded, ObservedAt: "today"},
	} {
		if code := kernel.CodeOf(knowledge.ValidateObservationBasis(invalid)); code != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("invalid basis %#v returned %s", invalid, code)
		}
	}
}
