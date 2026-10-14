package object_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestValidateBindingRejectsAmbiguousAndIncompleteDeclarations(t *testing.T) {
	cases := []*objectmodel.ValueSource{
		{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState}},
		{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: "snapshot", Runtime: "r", Protocol: "mcp", Operations: map[string]objectmodel.BindingOperation{"read": {Call: "x"}}}},
		{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState, DescriptorRef: "resource/r", Runtime: "r", Protocol: "mcp", Operations: map[string]objectmodel.BindingOperation{"read": {Call: "x"}}}},
	}
	for i, source := range cases {
		if code := kernel.CodeOf(objectmodel.ValidateValueSource(source)); code != kernel.ErrUsageInvalid {
			t.Fatalf("case %d must fail closed, got %s", i, code)
		}
	}
}
