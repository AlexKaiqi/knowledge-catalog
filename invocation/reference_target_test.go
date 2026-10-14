package invocation

import (
	"testing"

	"kc/invoke"
	"kc/kernel"
)

func TestReferenceCoordinatesAreBoundBeforeAuthorization(t *testing.T) {
	const ref = "urn:kc:object:kr%3A%2F%2Fsource%2Fa:table%2Forders#/columns/id"
	flags := map[string]invoke.FlagValue{"reference": ref}
	if err := NormalizeReferenceFlags(flags); err != nil || invoke.FlagString(flags, "repo") != "kr://source/a" || invoke.FlagString(flags, "object") != "table/orders" {
		t.Fatalf("authorization must use the actual source coordinates: %#v %v", flags, err)
	}
	for _, conflict := range []map[string]invoke.FlagValue{
		{"reference": ref, "repo": "kr://source/a"},
		{"reference": ref, "object": "table/orders"},
		{"reference": ref, "repo": "kr://source/b"},
		{"reference": ref, "object": "table/other"},
		{"reference": ref, "aspect": "private"},
		{"reference": ref, "include": []string{"private"}},
		{"reference": ref, "exclude": []string{"private"}},
		{"reference": ref, "member": "id"},
		{"reference": ref, "repo": "kr://source/a", referenceBindingFlag: true},
		{"reference": ref, "object": "table/orders", referenceBindingFlag: ref},
		{"reference": ref, "object": "table/orders", referenceBindingFlag: map[string]any{"reference": ref}},
	} {
		if err := NormalizeReferenceFlags(conflict); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("conflicting selection must refuse: %#v %v", conflict, err)
		}
	}
	dataset := map[string]invoke.FlagValue{"reference": ref, "dataset": "ks://published/orders", "commit": "fixed-source"}
	if err := NormalizeReferenceFlags(dataset); err != nil || invoke.FlagString(dataset, "repo") != "" || invoke.FlagString(dataset, "commit") != "" || invoke.FlagString(dataset, referenceCommitFlag) != "fixed-source" || invoke.FlagString(dataset, "object") != "table/orders" {
		t.Fatalf("Dataset scope must remain separate from source version: %#v %v", dataset, err)
	}
	if err := NormalizeReferenceFlags(dataset); err != nil || invoke.FlagString(dataset, referenceCommitFlag) != "fixed-source" {
		t.Fatalf("coordinate binding must be stable across transport and invocation: %#v %v", dataset, err)
	}
}

func TestBasicReadRequiresReferenceBeforeOpeningARepository(t *testing.T) {
	for _, flags := range []map[string]invoke.FlagValue{
		{},
		{"repo": "kr://source/a", "object": "table/orders"},
		{"dataset": "ks://published/orders", "object": "table/orders"},
	} {
		if _, err := VerbRead(&Invocation{Flags: flags}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("separate read selection reached a repository: %#v %v", flags, err)
		}
	}
}
