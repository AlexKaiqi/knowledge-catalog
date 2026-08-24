package main

import (
	"strings"
	"testing"
)

func TestTranslateMintsOpaqueStableIDsAndUniqueAddresses(t *testing.T) {
	snapshot := sourceSnapshot{
		SourceRef: "mysql://fixture/tpch",
		Tables:    []sourceTable{{Schema: "tpch", Name: "orders", Type: "BASE TABLE", Engine: "InnoDB"}},
		Columns: []sourceColumn{
			{Schema: "tpch", Table: "orders", Name: "o_orderkey", Ordinal: 1, Nullable: "NO", DataType: "int", ColumnType: "int"},
			{Schema: "tpch", Table: "orders", Name: "o_orderdate", Ordinal: 2, Nullable: "NO", DataType: "date", ColumnType: "date"},
		},
	}
	mapping := identityMap{Version: 1, Objects: map[string]string{}}
	first, err := translate(snapshot, &mapping)
	if err != nil {
		t.Fatal(err)
	}
	second, err := translate(snapshot, &mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || len(second) != 3 || len(mapping.Objects) != 3 {
		t.Fatalf("first=%d second=%d mapping=%d", len(first), len(second), len(mapping.Objects))
	}
	for i := range first {
		if first[i].Address != second[i].Address {
			t.Fatalf("identity changed at %d: %#v != %#v", i, first[i].Address, second[i].Address)
		}
		if strings.Contains(string(first[i].Address.ObjectID), "orders") {
			t.Fatalf("object_id leaked source name: %s", first[i].Address.ObjectID)
		}
	}
}

func TestTranslateRejectsColumnWithoutTable(t *testing.T) {
	mapping := identityMap{Version: 1, Objects: map[string]string{}}
	_, err := translate(sourceSnapshot{
		SourceRef: "mysql://fixture/tpch",
		Tables:    []sourceTable{{Schema: "tpch", Name: "orders"}},
		Columns:   []sourceColumn{{Schema: "tpch", Table: "missing", Name: "id", Ordinal: 1}},
	}, &mapping)
	if err == nil || !strings.Contains(err.Error(), "no table record") {
		t.Fatalf("got %v", err)
	}
}
