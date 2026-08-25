package knowledge_test

import (
	"testing"

	"kc/knowledge"
)

func TestParseSchemaRef(t *testing.T) {
	cases := []struct {
		ref  string
		ok   bool
		want knowledge.ParsedSchemaRef
	}{
		{ref: "", ok: false},
		{ref: "policy/A", ok: false},
		{ref: "schema/policy", ok: true, want: knowledge.ParsedSchemaRef{Object: "schema/policy"}},
		{ref: "schema/policy@c1", ok: true, want: knowledge.ParsedSchemaRef{Object: "schema/policy", Commit: "c1"}},
		{
			ref: "kc://acme/public/core/schema/policy", ok: true,
			want: knowledge.ParsedSchemaRef{Object: "schema/policy", Repository: "kr://acme/public/core"},
		},
		{
			ref: "kc://acme/public/core@deadbeef/schema/policy", ok: true,
			want: knowledge.ParsedSchemaRef{Object: "schema/policy", Commit: "deadbeef", Repository: "kr://acme/public/core"},
		},
		{ref: "kc://acme/public/core@deadbeef", ok: false},
		{ref: "kc://acme/public/core/not-schema", ok: false},
	}
	for _, tc := range cases {
		got, ok := knowledge.ParseSchemaRef(tc.ref)
		if ok != tc.ok {
			t.Fatalf("%q ok=%v want %v", tc.ref, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Fatalf("%q got %#v want %#v", tc.ref, got, tc.want)
		}
	}
	if !knowledge.IsSchemaObject("schema/policy") || knowledge.IsSchemaObject("policy/A") {
		t.Fatal("IsSchemaObject")
	}
}
