package format

import (
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestApplyOperationsSharesFinalRepresentationWithUnitAlgebra(t *testing.T) {
	old := Unit{ObjectID: "service/A", Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "health"}, Path: "custom/health.yaml", Value: map[string]any{"status": "old"}}
	old.Digest = kernel.CanonicalDigest(old.Value)
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: old.ObjectID, AspectName: "health", MemberKey: "primary"}
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: member, Value: map[string]any{"status": "new"}},
		{Op: objectmodel.OpRemove, Address: old.Address},
	}
	want := map[string]any{"health": map[string]any{"primary": map[string]any{"status": "new"}}}
	final, deleted, err := objectmodel.ApplyUnits(ToObjectUnits([]Unit{old}), operations, nil)
	if err != nil || len(deleted) != 1 || deleted[0] != old.Address {
		t.Fatalf("common algebra rejected consistent final conversion: final=%#v deleted=%#v error=%v", final, deleted, err)
	}
	coreValue, err := objectmodel.AssembleUnits(final)
	if err != nil || !reflect.DeepEqual(coreValue, want) {
		t.Fatalf("common algebra conversion: got %#v, want %#v, error %v", coreValue, want, err)
	}
	idx := NewTree()
	idx.Upsert(old)
	writes, removes := map[string]string{}, map[string]struct{}{}
	if err := ApplyOperations(idx, operations, nil, writes, removes); err != nil {
		t.Fatal(err)
	}
	value, err := Assemble(idx.ObjectUnits(old.ObjectID))
	if err != nil || !reflect.DeepEqual(value, want) {
		t.Fatalf("file conversion differs from the final contract: got %#v, want %#v, error %v", value, want, err)
	}
	if len(writes) != 1 || writes["service/A/health/primary.json"] == "" {
		t.Fatalf("new Member did not receive its default file position: %#v", writes)
	}
	if _, removed := removes["custom/health.yaml"]; !removed || len(removes) != 1 {
		t.Fatalf("conversion retained the old Aspect file: %#v", removes)
	}
}

func TestApplyOperationsRetainsExistingPositionAndReallocatesRecreatedUnit(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/A", AspectName: "health"}
	for _, recreate := range []bool{false, true} {
		name, wantPath := "update", "custom/health.yaml"
		if recreate {
			name, wantPath = "remove_then_put", "service/A/health.json"
		}
		t.Run(name, func(t *testing.T) {
			idx := NewTree()
			old := Unit{ObjectID: address.ObjectID, Address: address, Path: "custom/health.yaml", Value: map[string]any{"status": "old"}}
			old.Digest = kernel.CanonicalDigest(old.Value)
			idx.Upsert(old)
			operations := []objectmodel.Operation{}
			if recreate {
				operations = append(operations, objectmodel.Operation{Op: objectmodel.OpRemove, Address: address})
			}
			operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: address, Value: map[string]any{"status": "new"}})
			writes, removes := map[string]string{}, map[string]struct{}{}
			if err := ApplyOperations(idx, operations, nil, writes, removes); err != nil {
				t.Fatal(err)
			}
			if len(writes) != 1 || writes[wantPath] == "" || idx.Units[objectmodel.AddressKey(address)].Path != wantPath {
				t.Fatalf("wrong final position: writes=%#v unit=%#v", writes, idx.Units[objectmodel.AddressKey(address)])
			}
			_, removed := removes["custom/health.yaml"]
			if removed != recreate {
				t.Fatalf("old file removal=%v, want %v", removed, recreate)
			}
		})
	}
}
