package retrieval

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCompareScalarValuesPreservesDeclaredOrder(t *testing.T) {
	for _, tc := range []struct {
		name, kind  string
		left, right any
		want        int
	}{
		{"adjacent long", "long", json.Number("9007199254740992"), json.Number("9007199254740993"), -1},
		{"maximum long", "long", int64(math.MaxInt64), json.Number("9223372036854775806"), 1},
		{"exact floating integer", "long", float64(9007199254740992), json.Number("9007199254740993"), -1},
		{"integer exponent", "integer", json.Number("1e3"), json.Number("1000.0"), 0},
		{"fractional second", "timestamp", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00.000000001Z", -1},
		{"same instant", "datetime", "2026-01-01T08:00:00+08:00", "2026-01-01T00:00:00Z", 0},
		{"UTC expanded upper year", "timestamp", "9999-12-31T23:59:59-01:00", "10000-01-01T00:59:59Z", 0},
		{"UTC expanded lower year", "timestamp", "0000-01-01T00:00:00+01:00", "-0001-12-31T23:00:00Z", 0},
		{"negative decimal", "number", json.Number("-0.25"), -0.5, 1},
		{"boolean", "boolean", false, true, -1},
		{"string remains lexical", "string", "10", "2", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CompareScalarValues(tc.kind, tc.left, tc.right)
			if err != nil || got != tc.want {
				t.Fatalf("compare %v, %v as %s = %d, %v; want %d", tc.left, tc.right, tc.kind, got, err, tc.want)
			}
		})
	}
}

func TestScalarRejectsNonFiniteAndNonIntegralInputs(t *testing.T) {
	for _, raw := range []string{"NaN", "+Inf", "-Inf"} {
		if _, err := NormalizeScalarLiteral("number", raw); err == nil {
			t.Fatalf("non-finite scalar %s accepted", raw)
		}
	}
	if _, err := CompareScalarValues("long", float64(1.25), json.Number("1")); err == nil {
		t.Fatal("non-integral value accepted as integer")
	}
}

func TestNormalizeObjectReferenceScalars(t *testing.T) {
	for _, fieldType := range []string{"object_ref", "object_ref_list"} {
		got, err := NormalizeScalarLiteral(fieldType, "warehouse/table/orders")
		if err != nil {
			t.Fatalf("NormalizeScalarLiteral(%q): %v", fieldType, err)
		}
		if got != "warehouse/table/orders" {
			t.Fatalf("NormalizeScalarLiteral(%q) = %q", fieldType, got)
		}

		value, ok := NormalizeScalarValue(fieldType, "warehouse/table/orders")
		if !ok || value != "warehouse/table/orders" {
			t.Fatalf("NormalizeScalarValue(%q) = %q, %v", fieldType, value, ok)
		}
	}
}

func TestScalarTimeRejectsSubNanosecondLoss(t *testing.T) {
	if _, err := NormalizeScalarLiteral("timestamp", "2026-01-01T00:00:00.0000000001Z"); err == nil {
		t.Fatal("subnanosecond precision silently lost")
	}
	if got, err := NormalizeScalarLiteral("timestamp", "2026-01-01T00:00:00.0000000010Z"); err != nil || got != "2026-01-01T00:00:00.000000001Z" {
		t.Fatalf("equivalent zero tail: %q %v", got, err)
	}
	for _, raw := range []string{"9223372036854775808", "1e1000000000", "1.2", "1e-2"} {
		if _, err := NormalizeScalarLiteral("integer", raw); err == nil {
			t.Fatalf("invalid integer accepted %q", raw)
		}
	}
}
