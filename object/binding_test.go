package object_test

import (
	"encoding/json"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestValidateBindingRejectsAmbiguousAndIncompleteDeclarations(t *testing.T) {
	cases := []*objectmodel.ValueSource{
		{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{}},
		{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "schema/service.health"}},
		{Kind: "stream", Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/r"}},
		{Kind: objectmodel.ValueSourceSnapshot, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/r"}},
		{Kind: objectmodel.ValueSourceBinding},
	}
	for i, source := range cases {
		if code := kernel.CodeOf(objectmodel.ValidateValueSource(source)); code != kernel.ErrUsageInvalid {
			t.Fatalf("case %d must fail closed, got %s", i, code)
		}
	}
	if err := objectmodel.ValidateValueSource(&objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/r"}}); err != nil {
		t.Fatalf("descriptorRef binding rejected: %v", err)
	}
}

func TestValueSourceRejectsInlineAccessShape(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"binding","binding":{"mode":"state","runtime":"r","protocol":"resource-access/v1","operations":{"read":{"call":"x"}}}}`,
		`{"kind":"binding","binding":{"descriptorRef":"resource/r","origin":"https://x.example"}}`,
		`{"kind":"binding","origin":"https://x.example","binding":{"descriptorRef":"resource/r"}}`,
	} {
		var source objectmodel.ValueSource
		if code := kernel.CodeOf(json.Unmarshal([]byte(raw), &source)); code != kernel.ErrUsageInvalid {
			t.Fatalf("%s must be USAGE_INVALID, got %s", raw, code)
		}
	}
}

func TestBindingDeclarationDigestCoversUnitAndDescriptor(t *testing.T) {
	unit := objectmodel.DeclarationDigest("schema/service.health", &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/a"}})
	otherUnit := objectmodel.DeclarationDigest("schema/service.health", &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/b"}})
	descriptor := kernel.CanonicalDigest(map[string]any{"origin": "https://a.example"})
	otherDescriptor := kernel.CanonicalDigest(map[string]any{"origin": "https://b.example"})
	base := objectmodel.BindingDeclarationDigest(unit, descriptor)
	if base != objectmodel.BindingDeclarationDigest(unit, descriptor) {
		t.Fatal("declaration digest is not deterministic")
	}
	if base == objectmodel.BindingDeclarationDigest(otherUnit, descriptor) || base == objectmodel.BindingDeclarationDigest(unit, otherDescriptor) {
		t.Fatal("declaration digest must change when either the unit or the descriptor version changes")
	}
}
