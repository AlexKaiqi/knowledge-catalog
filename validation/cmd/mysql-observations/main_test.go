package main

import "testing"

func TestTranslateCreatesProfileSixJoinMembersAndAnnotation(t *testing.T) {
	source := "mysql://fixture/tpch"
	mapping := identityMap{Version: 1, Objects: map[string]string{
		source + "/table/tpch/lineitem":             "table-lineitem",
		source + "/table/tpch/orders":               "table-orders",
		source + "/column/tpch/lineitem/l_discount": "column-discount",
	}}
	joins := make([]joinEvidence, 6)
	for i := range joins {
		joins[i] = joinEvidence{
			Relation: "join-" + string(rune('a'+i)), ChildSchema: "tpch", ChildTable: "lineitem",
			ChildColumns: []string{"l_orderkey"}, ParentSchema: "tpch", ParentTable: "orders",
			ParentColumns: []string{"o_orderkey"}, ChildRowCount: 60175,
		}
	}
	units, err := translate(observationSnapshot{
		SourceRef:  source,
		Profile:    columnProfile{Schema: "tpch", Table: "lineitem", Column: "l_discount"},
		Joins:      joins,
		Annotation: annotation{Schema: "tpch", Table: "lineitem", Column: "l_discount"},
	}, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 8 {
		t.Fatalf("units=%d, want 8", len(units))
	}
	if units[0].Address.AspectName != "profile" || units[7].Address.AspectName != "annotation" {
		t.Fatalf("unexpected boundary units: %#v %#v", units[0], units[7])
	}
	for _, unit := range units[1:7] {
		if unit.Address.Kind != "Member" || unit.Address.AspectName != "joinEvidence" || unit.Address.MemberKey == "" {
			t.Fatalf("join unit is not an Address member: %#v", unit.Address)
		}
	}
}
