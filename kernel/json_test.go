package kernel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnmarshalJSONPreservesNumbersAcrossTypedAndUntypedContainers(t *testing.T) {
	var target struct {
		Small any              `json:"small"`
		Large any              `json:"large"`
		Typed int64            `json:"typed"`
		Exact json.Number      `json:"exact"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := UnmarshalJSON([]byte(`{"small":1e3,"large":9007199254740993,"typed":9223372036854775807,"exact":1.0,"rows":[{"fraction":0.125,"precise":0.123456789012345678901,"tiny":1e-999999999}]}`), &target); err != nil {
		t.Fatal(err)
	}
	if target.Small != float64(1000) || target.Large != json.Number("9007199254740993") || target.Typed != 9223372036854775807 || target.Exact != "1.0" {
		t.Fatalf("wrong numeric representations: %#v", target)
	}
	row := target.Rows[0]
	if row["fraction"] != float64(0.125) || row["precise"] != json.Number("0.123456789012345678901") || row["tiny"] != json.Number("1e-999999999") {
		t.Fatalf("nested exact numbers changed: %#v", row)
	}
}

func TestJSONHelpersRetainStrictInputBoundaries(t *testing.T) {
	for _, raw := range []string{`{"value":1} {"value":2}`, `1 2`, `{"value":1} garbage`} {
		var target any
		if err := UnmarshalJSON([]byte(raw), &target); err == nil || target != nil {
			t.Fatalf("invalid complete JSON changed target: %s => %#v, %v", raw, target, err)
		}
	}
	var target struct {
		Value any `json:"value"`
	}
	decoder := json.NewDecoder(strings.NewReader(`{"value":1,"unknown":2}`))
	decoder.DisallowUnknownFields()
	if err := DecodeJSON(decoder, &target); err == nil {
		t.Fatal("DecodeJSON disabled the caller's unknown-field policy")
	}
}
