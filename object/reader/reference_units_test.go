package reader_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestReferenceUnitsPlanWholeSelectedUnitBeforeLeafExists(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	guard.blocked["private/description.yaml"] = true
	guard.blocked["private/status.yaml"] = true
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: "/columns/order_id/not-yet-observed"}
	units, err := rd.ReadReferenceUnits(ref, at)
	want := map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil}
	if err != nil || len(units) != 1 || units[0].Address.MemberKey != "order_id" || !reflect.DeepEqual(units[0].Value, want) || units[0].Digest != kernel.CanonicalDigest(want) {
		t.Fatalf("whole selected unit before hydration: %#v %v", units, err)
	}
	if guard.reads["public/id.yaml"] != 1 || guard.reads["private/status.yaml"] != 0 || guard.reads["private/description.yaml"] != 0 {
		t.Fatalf("unit plan read siblings: %#v", guard.reads)
	}
}

type referenceUnitsRepo struct {
	snapshot.Store
	objectmodel.ReadStore
	units []objectmodel.Unit
	calls int
}

func (*referenceUnitsRepo) NativeObjectRepository() {}

func (r *referenceUnitsRepo) ReadReferenceUnits(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Unit, error) {
	r.calls++
	return r.units, nil
}

func TestNativeReferenceUnitsRejectProviderIdentitySelectionAndDigestDrift(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	base, err := rd.Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	unit := objectmodel.Unit{ObjectID: "table/orders", Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "order_id"}, Value: map[string]any{"type": "string"}, Digest: kernel.CanonicalDigest(map[string]any{"type": "string"})}
	for _, tc := range []struct {
		name      string
		mutate    func(*objectmodel.Unit)
		duplicate bool
		code      kernel.ErrorCode
	}{
		{"object", func(u *objectmodel.Unit) { u.ObjectID = "table/private" }, false, kernel.ErrPreconditionFailed},
		{"address-object", func(u *objectmodel.Unit) { u.Address.ObjectID = "table/private" }, false, kernel.ErrPreconditionFailed},
		{"sibling", func(u *objectmodel.Unit) { u.Address.MemberKey = "status" }, false, kernel.ErrPreconditionFailed},
		{"digest", func(u *objectmodel.Unit) { u.Digest = "false-summary" }, false, kernel.ErrPreconditionFailed},
		{"duplicate", func(*objectmodel.Unit) {}, true, kernel.ErrObjectIDConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := unit
			tc.mutate(&candidate)
			repo := &referenceUnitsRepo{Store: guard.Store, ReadStore: base, units: []objectmodel.Unit{candidate}}
			if tc.duplicate {
				repo.units = append(repo.units, candidate)
			}
			native := reader.NewReader(nil)
			view, err := native.Wrap(repo, kernel.ErrCapabilityUnsatisfied)
			if err != nil {
				t.Fatal(err)
			}
			scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return view, nil }, reader.DatasetPin{SetID: "native-units", Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at})})
			scoped, err := scope.Member(repo.ID())
			if err != nil {
				t.Fatal(err)
			}
			_, err = scoped.(objectmodel.ReferenceUnitsReader).ReadReferenceUnits(objectmodel.Reference{Repository: repo.ID(), Object: "table/orders", Locator: "/columns/order_id/type"}, at)
			if kernel.CodeOf(err) != tc.code {
				t.Fatalf("provider drift not rejected: %v", err)
			}
			if repo.calls != 1 {
				t.Fatalf("provider reads %d", repo.calls)
			}
		})
	}
}
