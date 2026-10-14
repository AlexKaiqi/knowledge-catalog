package service_test

import (
	"context"
	liveservice "kc/liveaccess/service"
	"testing"

	"kc/kernel"
	serving "kc/liveaccess"
	objectmodel "kc/object"
)

func TestBoundObservationHonorsActualSchemaWithoutInventingNullValue(t *testing.T) {
	base, _, _, address := setupServing(t)
	for _, test := range []struct {
		name    string
		value   any
		invalid bool
	}{
		{"invalid-declared-scalar", map[string]any{"status": 42}, true},
		{"valid-declared-scalar", map[string]any{"status": "healthy"}, false},
		{"valid-missing-optional-field", map[string]any{}, false},
		{"observed-null", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			lookup := &stateLookup{result: serving.StateObservation{Value: test.value, Basis: objectmodel.ObservationBasis{
				BindingGeneration: "generation", Consistency: objectmodel.ObservationLatestOnly, ObservedAt: "2026-10-13T00:00:00Z",
			}}}
			service := liveservice.Open(base, lookup, kernel.IdentityContext{Principal: "agent"})
			result, err := service.ReadAddress(context.Background(), address)
			if test.invalid {
				assertFailedUnitRead(t, result, err, address, kernel.ErrSchemaInstanceInvalid)
				return
			}
			if err != nil || len(result) != 1 || len(result[0].Observations) != 1 {
				t.Fatalf("legal observation was rejected: %#v %v", result, err)
			}
			if test.value == nil && result[0].Value != nil {
				t.Fatalf("observed null was replaced: %#v", result)
			}
		})
	}
}
