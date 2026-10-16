package object

import (
	"reflect"
	"testing"

	"kc/kernel"
)

func maintenanceBindingUnit(ref ObjectID) Unit {
	return Unit{ObjectID: "table/orders", Address: Address{Kind: KindMember, ObjectID: "table/orders", AspectName: "runtime", MemberKey: "count"},
		SchemaRef: "schema/count", ValueSource: &ValueSource{Kind: ValueSourceBinding, Binding: &BindingDeclaration{DescriptorRef: ref}}, Digest: kernel.CanonicalDigest(nil)}
}

func TestMaintenanceDigestUsesOnlyLocalBindingDeclaration(t *testing.T) {
	u := maintenanceBindingUnit("resource/warehouse")
	want := DeclarationDigest(u.SchemaRef, u.ValueSource)
	if got := MaintenanceDigest(u.Digest, u.SchemaRef, u.ValueSource); got != want || got == u.Digest {
		t.Fatalf("binding basis %s; declaration=%s content=%s", got, want, u.Digest)
	}
	if got := MaintenanceDigest(u.Digest, "schema/other", u.ValueSource); got == want {
		t.Fatal("Schema adoption change preserved the binding maintenance basis")
	}
	other := maintenanceBindingUnit("resource/pipeline")
	if got := MaintenanceDigest(other.Digest, other.SchemaRef, other.ValueSource); got == want {
		t.Fatal("binding target change preserved the maintenance basis")
	}
	for _, source := range []*ValueSource{nil, {Kind: ValueSourceSnapshot}} {
		if got := MaintenanceDigest("static-content", "schema/any", source); got != "static-content" {
			t.Fatalf("Snapshot compatibility changed: %s", got)
		}
	}
}

func TestWholeObjectMaintenanceBasisIsStableAndCoversEveryUnit(t *testing.T) {
	bound := maintenanceBindingUnit("resource/warehouse")
	static := Unit{ObjectID: bound.ObjectID, Address: Address{Kind: KindAspect, ObjectID: bound.ObjectID, AspectName: "description"}, Value: "orders", Digest: kernel.CanonicalDigest("orders")}
	units := []Unit{bound, static}
	basis, err := UnitSetMaintenanceDigest(units)
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := UnitSetMaintenanceDigest([]Unit{static, bound})
	if err != nil || reversed != basis || !reflect.DeepEqual(units, []Unit{bound, static}) {
		t.Fatalf("order or caller units changed: %s %v %#v", reversed, err, units)
	}
	for name, changed := range map[string][]Unit{
		"binding": {maintenanceBindingUnit("resource/pipeline"), static},
		"static":  {bound, {ObjectID: static.ObjectID, Address: static.Address, Value: "changed", Digest: kernel.CanonicalDigest("changed")}},
		"removed": {bound},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := UnitSetMaintenanceDigest(changed)
			if err != nil || got == basis {
				t.Fatalf("changed whole object kept basis %s: %v", got, err)
			}
		})
	}
	root := Address{Kind: KindEntity, ObjectID: bound.ObjectID}
	_, _, err = ApplyUnits([]Unit{bound, {ObjectID: static.ObjectID, Address: static.Address, Value: "changed", Digest: kernel.CanonicalDigest("changed")}},
		[]Operation{{Op: OpRemove, Address: root, Precondition: &Precondition{Type: IfDigestEquals, Digest: basis}}}, nil)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("changed static sibling escaped whole-object condition: %v", err)
	}
}

func TestStaticMaintenanceBasisKeepsExistingContentCompatibility(t *testing.T) {
	units := []Unit{
		{ObjectID: "table/orders", Address: Address{Kind: KindAspect, ObjectID: "table/orders", AspectName: "owner"}, Value: "ops", Digest: kernel.CanonicalDigest("ops")},
		{ObjectID: "table/orders", Address: Address{Kind: KindAspect, ObjectID: "table/orders", AspectName: "description"}, Value: "orders", Digest: kernel.CanonicalDigest("orders")},
	}
	assembled, _ := AssembleUnits(units)
	basis, err := UnitSetMaintenanceDigest(units)
	if err != nil || basis != kernel.CanonicalDigest(assembled) {
		t.Fatalf("all-Snapshot object changed basis: %s %v", basis, err)
	}
	for _, operation := range []Operation{
		{Op: OpPut, Address: units[0].Address, Value: "new", Precondition: &Precondition{Type: IfDigestEquals, Digest: units[0].Digest}},
		{Op: OpRemove, Address: units[0].Address, Precondition: &Precondition{Type: IfDigestEquals, Digest: units[0].Digest}},
		{Op: OpRemove, Address: Address{Kind: KindEntity, ObjectID: "table/orders"}, Precondition: &Precondition{Type: IfDigestEquals, Digest: basis}},
	} {
		if _, _, err := ApplyUnits(units, []Operation{operation}, nil); err != nil {
			t.Fatalf("existing static condition rejected: %v", err)
		}
	}
}
