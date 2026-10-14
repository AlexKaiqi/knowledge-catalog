package object_test

import (
	"testing"

	objectmodel "kc/object"
)

func TestParseSchemaRef(t *testing.T) {
	cases := []struct {
		ref  string
		ok   bool
		want objectmodel.ParsedSchemaRef
	}{
		{ref: "", ok: false},
		{ref: "policy/A", ok: false},
		{ref: "schema/policy", ok: true, want: objectmodel.ParsedSchemaRef{Object: "schema/policy"}},
		{ref: "schema/policy@c1", ok: true, want: objectmodel.ParsedSchemaRef{Object: "schema/policy", Commit: "c1"}},
		{
			ref: "kc://acme/public/core/schema/policy", ok: true,
			want: objectmodel.ParsedSchemaRef{Object: "schema/policy", Repository: "kr://acme/public/core"},
		},
		{
			ref: "kc://acme/public/core@deadbeef/schema/policy", ok: true,
			want: objectmodel.ParsedSchemaRef{Object: "schema/policy", Commit: "deadbeef", Repository: "kr://acme/public/core"},
		},
		{ref: "kc://acme/public/core@deadbeef", ok: false},
		{ref: "kc://acme/public/core/not-schema", ok: false},
	}
	for _, tc := range cases {
		got, ok := objectmodel.ParseSchemaRef(tc.ref)
		if ok != tc.ok {
			t.Fatalf("%q ok=%v want %v", tc.ref, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Fatalf("%q got %#v want %#v", tc.ref, got, tc.want)
		}
	}
	if !objectmodel.IsSchemaObject("schema/policy") || objectmodel.IsSchemaObject("policy/A") {
		t.Fatal("IsSchemaObject")
	}
}
