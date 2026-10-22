package object

import (
	"testing"

	"kc/kernel"
)

func TestLocalStateSeparatesValueDeclarationAndUnitComposition(t *testing.T) {
	id := ObjectID("table/orders")
	unit := Unit{ObjectID: id, Address: Address{Kind: KindAspect, ObjectID: id, AspectName: "definition"},
		Value: map[string]any{"name": "orders", "owner": "trade"}, SchemaRef: "schema/table"}
	unit.Digest = kernel.CanonicalDigest(unit.Value)
	original := LocalStateDigest(DeclareUnits([]Unit{unit}))
	for name, change := range map[string]func(*Unit){
		"unprojected field":  func(u *Unit) { u.Digest = kernel.CanonicalDigest(map[string]any{"name": "orders", "owner": "ops"}) },
		"schema declaration": func(u *Unit) { u.SchemaRef = "schema/other" },
		"binding declaration": func(u *Unit) {
			u.ValueSource = &ValueSource{Kind: ValueSourceBinding, Binding: &BindingDeclaration{DescriptorRef: "resource/orders"}}
		},
		"unit address": func(u *Unit) { u.Address.AspectName = "description" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := unit
			change(&changed)
			if LocalStateDigest(DeclareUnits([]Unit{changed})) == original {
				t.Fatal("local state lost a persistent change")
			}
		})
	}
	if LocalStateDigest(nil) == original {
		t.Fatal("removing the last unit preserved state")
	}
	other := unit
	other.Address.AspectName = "runbook"
	a, b := DeclareUnits([]Unit{unit, other}), DeclareUnits([]Unit{other, unit})
	if LocalStateDigest(a) != LocalStateDigest(b) || LocalStateDigest(a) == original {
		t.Fatal("unit membership or order changed equality")
	}
	// Read-time descriptor interpretation may replace DeclarationDigest. Local
	// state must still compare the local declaration, never that effective one.
	a = DeclareUnits([]Unit{unit})
	a[0].DeclarationDigest = "effective-descriptor-version"
	a[0].Provenance = &ProvenanceEnvelope{}
	if LocalStateDigest(a) != original {
		t.Fatal("interpretation or provenance entered local state")
	}
	unit.PathHint = "moved/definition.yaml"
	if LocalStateDigest(DeclareUnits([]Unit{unit})) != original {
		t.Fatal("path entered local state")
	}
	maintenance, err := UnitSetMaintenanceDigest([]Unit{unit})
	if err != nil || maintenance != kernel.CanonicalDigest(map[string]any{"definition": unit.Value}) {
		t.Fatalf("maintenance equality changed: %s %v", maintenance, err)
	}
}
