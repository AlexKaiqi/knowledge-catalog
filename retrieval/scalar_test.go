package retrieval

import "testing"

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
