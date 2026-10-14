package object_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestValidateObservationBasis(t *testing.T) {
	valid := objectmodel.ObservationBasis{
		BindingGeneration: "orders-health-v3",
		Consistency:       objectmodel.ObservationRepeatable,
		SourceRevision:    "42",
		ObservedAt:        "2026-08-27T08:30:00.123Z",
	}
	if err := objectmodel.ValidateObservationBasis(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []objectmodel.ObservationBasis{
		{Consistency: objectmodel.ObservationRepeatable, ObservedAt: "2026-08-27T08:30:00Z"},
		{BindingGeneration: "g1", Consistency: "eventual", ObservedAt: "2026-08-27T08:30:00Z"},
		{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly},
		{BindingGeneration: "g1", Consistency: objectmodel.ObservationBounded, ObservedAt: "today"},
	} {
		if code := kernel.CodeOf(objectmodel.ValidateObservationBasis(invalid)); code != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("invalid basis %#v returned %s", invalid, code)
		}
	}
}
