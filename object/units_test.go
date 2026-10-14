package object

import (
	"reflect"
	"testing"
)

func TestProviderNeutralUnitAlgebraHasNoFileStorageShape(t *testing.T) {
	typ := reflect.TypeOf(Unit{})
	for _, forbidden := range []string{"Path", "Filename", "Extension", "Content"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("provider-neutral Unit exposes file field %s", forbidden)
		}
	}
}

func TestProviderNeutralApplyAndAssemblyUseOnlyAddresses(t *testing.T) {
	final, deleted, err := ApplyUnits(nil, []Operation{
		{Op: OpPut, Address: Address{Kind: KindAspect, ObjectID: "dataset/T", AspectName: "structure"}, Value: map[string]any{"columns": 2}},
		{Op: OpPut, Address: Address{Kind: KindAspect, ObjectID: "dataset/T", AspectName: "owner"}, Value: map[string]any{"name": "ops"}},
	}, nil)
	if err != nil || len(final) != 2 || len(deleted) != 0 {
		t.Fatalf("apply = %#v deleted=%#v err=%v", final, deleted, err)
	}
	value, err := AssembleUnits(final)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	if record["structure"] == nil || record["owner"] == nil {
		t.Fatalf("assembled value = %#v", value)
	}
}
