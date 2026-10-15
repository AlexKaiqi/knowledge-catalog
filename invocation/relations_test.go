package invocation

import (
	"testing"

	"kc/kernel"
)

func TestRelationEndpointFlagTakesObjectIDOnly(t *testing.T) {
	id, err := RelationEndpointFlag("table/orders")
	if err != nil || id != "table/orders" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	for _, invalid := range []string{
		"orders",
		"urn:kc:object:table/orders",
		"kc://acme/public/core/table/orders",
		"kc://acme/public/core@c1/Table:orders",
	} {
		if _, err := RelationEndpointFlag(invalid); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("%q: got %v, want USAGE_INVALID", invalid, err)
		}
	}
}
