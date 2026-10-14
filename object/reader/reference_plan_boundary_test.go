package reader_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestReferencePlanUsesMaintenanceMetadataWithoutReadingBodies(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	for _, path := range []string{"public/id.yaml", "private/description.yaml", "private/status.yaml"} {
		guard.blocked[path] = true
	}
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id/type"}
	coordinates, err := rd.PlanReference(ref, at)
	want := []objectmodel.Address{{Kind: objectmodel.KindMember, ObjectID: ref.Object, AspectName: "columns", MemberKey: "order_id"}}
	if err != nil || !reflect.DeepEqual(coordinates, want) {
		t.Fatalf("metadata plan: %#v %v", coordinates, err)
	}
	ref.Locator = "/virtual-facet/status"
	coordinates, err = rd.PlanReference(ref, at)
	if err != nil || len(coordinates) != 0 {
		t.Fatalf("planner invented maintenance coordinates: %#v %v", coordinates, err)
	}
	for _, path := range []string{"public/id.yaml", "private/description.yaml", "private/status.yaml"} {
		if guard.reads[path] != 0 {
			t.Fatalf("planner accessed body %s", path)
		}
	}
}

type nativeReferencePlanRepo struct {
	*referenceUnitsRepo
	coordinates []objectmodel.Address
}

func (r *nativeReferencePlanRepo) PlanReference(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Address, error) {
	return r.coordinates, nil
}

func TestNativeReferencePlanRejectsUnprovenOrConflictingCoordinates(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	base, err := rd.Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"}
	for _, tc := range []struct {
		name, locator string
		addresses     []objectmodel.Address
	}{
		{"different-object", "/columns/order_id", []objectmodel.Address{{Kind: objectmodel.KindMember, ObjectID: "private/object", AspectName: "columns", MemberKey: "order_id"}}},
		{"invalid-kind", "/columns/order_id", []objectmodel.Address{{Kind: objectmodel.KindEntity, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"}}},
		{"unselected-member", "/columns/order_id", []objectmodel.Address{{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "status"}}},
		{"duplicate", "/columns/order_id", []objectmodel.Address{member, member}},
		{"mixed-root-and-parts", "", []objectmodel.Address{{Kind: objectmodel.KindEntity, ObjectID: "table/orders"}, member}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &nativeReferencePlanRepo{referenceUnitsRepo: &referenceUnitsRepo{Store: guard.Store, ReadStore: base}, coordinates: tc.addresses}
			view, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
			if err != nil {
				t.Fatal(err)
			}
			_, err = view.(objectmodel.ReferencePlanner).PlanReference(objectmodel.Reference{Repository: raw.ID(), Object: "table/orders", Locator: tc.locator}, at)
			if err == nil {
				t.Fatalf("invalid provider coordinates passed metadata authorization planning: %#v", tc.addresses)
			}
		})
	}
}
