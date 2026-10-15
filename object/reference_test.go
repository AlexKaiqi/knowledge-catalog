package object_test

import (
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/object"
)

func TestObjectReferenceKeepsIdentitySeparateFromPointer(t *testing.T) {
	for _, tc := range []struct {
		ref  object.Reference
		want string
	}{
		{object.Reference{Object: "table/orders"}, "urn:kc:object:table/orders"},
		{object.Reference{Object: "table/订单:%?#/空 格", Locator: "/columns/order_id/constraints/nullable"}, ""},
		{object.Reference{Object: "table/a+b", Locator: "/display name/中文/~01"}, ""},
		{object.Reference{Object: "kc/concept/object", Locator: "/definition"}, "urn:kc:object:kc/concept/object#/definition"},
	} {
		raw, err := object.FormatReference(tc.ref)
		if err != nil {
			t.Fatal(err)
		}
		if tc.want != "" && raw != tc.want {
			t.Fatalf("canonical reference: got %s want %s", raw, tc.want)
		}
		got, err := object.ParseReference(raw)
		if err != nil || got != tc.ref {
			t.Fatalf("reference round trip: raw=%s got=%#v want=%#v error=%v", raw, got, tc.ref, err)
		}
	}
	// The bound repository is request scope, never URN text.
	raw, err := object.FormatReference(object.Reference{Repository: "kr://scene/object", Object: "table/orders"})
	if err != nil || raw != "urn:kc:object:table/orders" {
		t.Fatalf("repository leaked into the URN: %s %v", raw, err)
	}
	base := "urn:kc:object:table/orders"
	for _, raw := range []string{base, base + "#"} {
		got, err := object.ParseReference(raw)
		if err != nil || got.Locator != "" || got.Repository != "" || got.Object != "table/orders" {
			t.Fatalf("root reference: %#v %v", got, err)
		}
	}
	got, err := object.ParseReference(base + "#/a%2Fb")
	if err != nil || got.Locator != "/a/b" {
		t.Fatalf("URI decoding must precede pointer decoding: %#v %v", got, err)
	}
	for _, raw := range []string{
		"", "kc://scene/object/table/orders", "urn:kc:object:", "urn:kc:object:orders",
		"urn:kc:object:/orders", "urn:kc:object:table/", "urn:kc:object:table//orders",
		"urn:kc:object:table%2Forders", base + ":ambiguous",
		"urn:kc:object:kr%ZZ/orders", "urn:kc:object:kr%00/orders",
		base + "?query", base + "#not-a-pointer", base + "#/bad~2",
		base + "#/bad~", base + "#/bad%ZZ", base + "#/a#b",
	} {
		if _, err := object.ParseReference(raw); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("invalid reference %q: %v", raw, err)
		}
	}
}

func TestRepositoryQualifiedURNIsRejected(t *testing.T) {
	legacy := "urn:kc:object:" + "kr%3A%2F%2Fscene%2Fobject" + ":"
	for _, raw := range []string{
		legacy + "table%2Forders",
		legacy + "table%2Forders#/columns/id",
	} {
		_, err := object.ParseReference(raw)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "retired") {
			t.Fatalf("retired repository-qualified URN must be refused explicitly: %q %v", raw, err)
		}
	}
}

func TestObjectIDRequiresNamespace(t *testing.T) {
	for _, id := range []object.ObjectID{"trade/order-service", "dw.ods/orders", "kc/concept/object"} {
		if err := object.ValidateObjectID(id); err != nil {
			t.Fatalf("namespaced id %s rejected: %v", id, err)
		}
	}
	if object.Namespace("dw.ods/orders") != "dw.ods" {
		t.Fatal("namespace is the first segment")
	}
	for _, id := range []object.ObjectID{"", "orders", "/orders", "trade/", "trade//x", " /x"} {
		if err := object.ValidateObjectID(id); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("id %q must be refused: %v", id, err)
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
		if _, err := object.SelectPointer(value, pointer); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
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
	ref, err := object.ParseReference("URN:KC:object:Table/Orders#/Column/Id")
	if err != nil || ref.Object != "Table/Orders" || ref.Locator != "/Column/Id" {
		t.Fatalf("URN prefix casing changed namespace-specific identity: %#v %v", ref, err)
	}
	raw, err := object.FormatReference(ref)
	if err != nil || !strings.HasPrefix(raw, "urn:kc:object:") {
		t.Fatalf("canonical reference prefix: %s %v", raw, err)
	}
}
