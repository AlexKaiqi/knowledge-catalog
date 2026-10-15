package invocation

import (
	"testing"

	"kc/invoke"
	"kc/kernel"
)

func TestReferenceCoordinatesAreBoundBeforeAuthorization(t *testing.T) {
	const ref = "urn:kc:object:table/orders#/columns/id"
	scoped := map[string]invoke.FlagValue{"reference": ref, "repo": "kr://source/a"}
	if err := NormalizeReferenceFlags(scoped); err != nil || invoke.FlagString(scoped, "repo") != "kr://source/a" || invoke.FlagString(scoped, "object") != "table/orders" {
		t.Fatalf("authorization must use the named Repository scope: %#v %v", scoped, err)
	}
	if bound, _ := referenceFromFlags(scoped); bound == nil || bound.Repository != "kr://source/a" || bound.Locator != "/columns/id" {
		t.Fatalf("bound reference lost its scope: %#v", bound)
	}
	unscoped := map[string]invoke.FlagValue{"reference": ref}
	if err := NormalizeReferenceFlags(unscoped); err != nil || invoke.FlagString(unscoped, "repo") != "" || invoke.FlagString(unscoped, "object") != "table/orders" {
		t.Fatalf("an unscoped URN must stay unresolved until the request scope is searched: %#v %v", unscoped, err)
	}
	for _, conflict := range []map[string]invoke.FlagValue{
		{"reference": ref, "object": "table/orders"},
		{"reference": ref, "object": "table/other"},
		{"reference": ref, "aspect": "private"},
		{"reference": ref, "include": []string{"private"}},
		{"reference": ref, "exclude": []string{"private"}},
		{"reference": ref, "member": "id"},
		{"reference": ref, "commit": "c1"},
		{"reference": ref, "dataset": "ks://published/orders", "repo": "kr://source/a"},
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
