package kernel

import (
	"encoding/json"
	"testing"
)

func TestCanonicalDigestPreservesExactJSONNumbers(t *testing.T) {
	type document struct {
		Value int64 `json:"value"`
	}
	const first int64 = 9007199254740992
	const second int64 = 9007199254740993
	for _, tc := range []struct {
		name        string
		left, right any
	}{
		{"struct", document{first}, document{second}},
		{"raw", json.RawMessage(`{"value":9007199254740992}`), json.RawMessage(`{"value":9007199254740993}`)},
		{"small-nonzero", json.Number("1e-999999999"), json.Number("0")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if CanonicalDigest(tc.left) == CanonicalDigest(tc.right) {
				t.Fatal("different exact JSON numbers collapsed to one digest")
			}
		})
	}
	for _, equivalent := range []any{
		document{second},
		json.RawMessage(`{"value":9007199254740993}`),
		map[string]any{"value": json.Number("9007199254740993.0")},
		map[string]any{"value": json.Number("9.007199254740993e15")},
	} {
		if CanonicalDigest(equivalent) != CanonicalDigest(map[string]any{"value": second}) {
			t.Fatalf("equivalent representation changed exact integer digest: %#v", equivalent)
		}
	}
	for _, spellings := range [][]string{
		{"1", "1.0", "1e0"}, {"1000", "1e3", "10.00e2"},
		{"1e1000000000", "10e999999999"}, {"1e-1000000000", "10e-1000000001"},
		{"0", "-0", "0e99999999999999999999999999999"},
	} {
		want := CanonicalDigest(json.Number(spellings[0]))
		for _, raw := range spellings {
			if CanonicalDigest(json.Number(raw)) != want {
				t.Fatalf("equivalent JSON number %s changed digest", raw)
			}
		}
	}
}
