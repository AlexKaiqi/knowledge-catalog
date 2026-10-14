package object_test

import (
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/object"
)

func TestObjectReferenceKeepsIdentitySeparateFromPointer(t *testing.T) {
	for _, ref := range []object.Reference{
		{Repository: "kr://scene/knowledge", Object: "table/orders"},
		{Repository: "kr://仓/100%:a?b#c", Object: "table/订单:%?#/空 格", Locator: "/columns/order_id/constraints/nullable"},
		{Repository: "kr://scene/knowledge", Object: "table/a+b", Locator: "/display name/中文/~01"},
	} {
		raw, err := object.FormatReference(ref)
		if err != nil {
			t.Fatal(err)
		}
		got, err := object.ParseReference(raw)
		if err != nil || got != ref {
			t.Fatalf("reference round trip: raw=%s got=%#v want=%#v error=%v", raw, got, ref, err)
		}
	}
	base := "urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:table%2Forders"
	for _, raw := range []string{base, base + "#"} {
		got, err := object.ParseReference(raw)
		if err != nil || got.Locator != "" || got.Repository != "kr://scene/knowledge" || got.Object != "table/orders" {
			t.Fatalf("root reference: %#v %v", got, err)
		}
	}
	got, err := object.ParseReference(base + "#/a%2Fb")
	if err != nil || got.Locator != "/a/b" {
		t.Fatalf("URI decoding must precede pointer decoding: %#v %v", got, err)
	}
	for _, raw := range []string{
		"", "kc://scene/knowledge/table/orders", "urn:kc:object::orders",
		"urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:", base + ":ambiguous",
		"urn:kc:object:kr%ZZ:orders", "urn:kc:object:kr%00:orders",
		base + "?query", base + "#not-a-pointer", base + "#/bad~2",
		base + "#/bad~", base + "#/bad%ZZ", base + "#/a#b",
	} {
		if _, err := object.ParseReference(raw); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("invalid reference %q: %v", raw, err)
		}
	}
}

func TestPointerSelectionDistinguishesKeysArraysAndMissingValues(t *testing.T) {
	value := map[string]any{
		"": "empty key", "a/b": "one key", "a": map[string]any{"b": "nested"},
		"a.b": "literal dot", "~1": "decode once", "nil": nil, "false": false,
		"array": []any{"first", map[string]any{"中文": "第二项"}},
	}
	for _, tc := range []struct {
		pointer string
		want    any
	}{
		{"", value}, {"/", "empty key"}, {"/a~1b", "one key"}, {"/a/b", "nested"},
		{"/a.b", "literal dot"}, {"/~01", "decode once"}, {"/nil", nil}, {"/false", false},
		{"/array/0", "first"}, {"/array/1/中文", "第二项"},
	} {
		got, err := object.SelectPointer(value, tc.pointer)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pointer %q: got=%#v want=%#v error=%v", tc.pointer, got, tc.want, err)
		}
	}
	for _, pointer := range []string{"/missing", "/nil/child", "/array/2", "/array/-"} {
		if _, err := object.SelectPointer(value, pointer); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			t.Errorf("unresolved pointer %q: %v", pointer, err)
		}
	}
	for _, pointer := range []string{"no-slash", "/bad~2", "/bad~", "/array/01", "/array/+1"} {
		if _, err := object.SelectPointer(value, pointer); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("invalid pointer %q: %v", pointer, err)
		}
	}
}

func TestReferenceURNPrefixIsCaseInsensitiveWithoutChangingIdentity(t *testing.T) {
	ref, err := object.ParseReference("URN:KC:object:kr%3A%2F%2FScene%2FKnowledge:Table%2FOrders#/Column/Id")
	if err != nil || ref.Repository != "kr://Scene/Knowledge" || ref.Object != "Table/Orders" || ref.Locator != "/Column/Id" {
		t.Fatalf("URN prefix casing changed namespace-specific identity: %#v %v", ref, err)
	}
	raw, err := object.FormatReference(ref)
	if err != nil || !strings.HasPrefix(raw, "urn:kc:object:") {
		t.Fatalf("canonical reference prefix: %s %v", raw, err)
	}
}
