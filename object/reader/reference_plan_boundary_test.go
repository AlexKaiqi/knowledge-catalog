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

func TestDatasetReferencePlanKeepsOnlyPublishedCoordinates(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	for _, path := range []string{"public/id.yaml", "private/description.yaml", "private/status.yaml"} {
		guard.blocked[path] = true
	}
	serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return rd.Require(id, kernel.ErrObjectRefUnresolved)
	}, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at}, Items: []reader.DatasetItem{
		{Repository: guard.ID(), Commit: at, Kind: "file", File: "public/id.yaml", Target: "id.yaml"},
	}})
	member, err := serving.Member(guard.ID())
	if err != nil {
		t.Fatal(err)
	}
	planner := member.(objectmodel.ReferencePlanner)
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders"}
	want := []objectmodel.Address{{Kind: objectmodel.KindMember, ObjectID: ref.Object, AspectName: "columns", MemberKey: "order_id"}}
	for _, locator := range []string{"", "/columns", "/columns/order_id/type"} {
		ref.Locator = locator
		got, err := planner.PlanReference(ref, at)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Dataset metadata plan %q must exclude unpublished coordinates: %#v %v", locator, got, err)
		}
	}
	ref.Locator = "/description"
	if got, err := planner.PlanReference(ref, at); err != nil || len(got) != 0 {
		t.Errorf("unselected content must not contribute a holding: %#v %v", got, err)
	}
	invalid := objectmodel.Reference{Repository: guard.ID(), Object: "note/missing", Locator: "not-a-pointer"}
	if _, err := planner.PlanReference(invalid, at); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Errorf("an absent holding must not bypass reference validation: %v", err)
	}
	for path := range guard.blocked {
		if guard.reads[path] != 0 {
			t.Errorf("Dataset planning read body %s", path)
		}
	}
}

type nativeReferencePlanRepo struct {
	*referenceUnitsRepo
	coordinates []objectmodel.Address
	paths       []string
}

func (r *nativeReferencePlanRepo) PlanReference(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Address, error) {
	return r.coordinates, nil
}

func (r *nativeReferencePlanRepo) ObjectUnitPaths(objectmodel.ObjectID, kernel.CommitID) ([]string, error) {
	return append([]string(nil), r.paths...), nil
}

func TestNativeDatasetReferencePlanNeedsMetadataPathCoordinates(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	base, err := rd.Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"public/id.yaml", "private/description.yaml"} {
		guard.blocked[path] = true
	}
	raw := &nativeReferencePlanRepo{
		referenceUnitsRepo: &referenceUnitsRepo{Store: guard.Store, ReadStore: base},
		coordinates: []objectmodel.Address{
			{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"},
			{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "description"},
		},
		paths: []string{"public/id.yaml", "private/description.yaml"},
	}
	serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) {
		return reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	}, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): at}, Items: []reader.DatasetItem{
		{Repository: raw.ID(), Commit: at, Kind: "file", File: "public/id.yaml", Target: "id.yaml"},
	}})
	member, err := serving.Member(raw.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = member.(objectmodel.ReferencePlanner).PlanReference(objectmodel.Reference{Repository: raw.ID(), Object: "table/orders"}, at)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unrelated address and path lists cannot prove slice membership: %v", err)
	}
	for path := range guard.blocked {
		if guard.reads[path] != 0 {
			t.Errorf("native planning read body %s", path)
		}
	}
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
