package capability

import (
	"encoding/json"
	"math"
	"testing"

	retrieval "kc/discovery"
)

func numericManifest(t *testing.T, kind string, values []string) Manifest {
	t.Helper()
	manifest := DefaultManifest()
	manifest.Builds[0].Options = append(manifest.Builds[0].Options, BuildOption{Name: "weight", Type: kind, Values: values})
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var published Manifest
	if err := json.Unmarshal(encoded, &published); err != nil {
		t.Fatal(err)
	}
	return published
}

func TestManifestNumericOptionsMatchPublishedTypes(t *testing.T) {
	for _, kind := range []string{"number", "integer"} {
		manifest := numericManifest(t, kind, nil)
		for _, tc := range []struct {
			name            string
			value           any
			number, integer bool
		}{
			{"signed", int64(-2), true, true},
			{"unsigned", uint64(18446744073709551615), true, true},
			{"float32", float32(2), true, true},
			{"fraction", 2.5, true, false},
			{"decimal-integer", json.Number("2.0"), true, true},
			{"large-integer", json.Number("9007199254740993"), true, true},
			{"precise-fraction", json.Number("9007199254740993.1"), true, false},
			{"shifted-integer", json.Number("120e-1"), true, true},
			{"shifted-fraction", json.Number("12e-1"), true, false},
			{"huge-exponent", json.Number("1e999999999999999999999999"), true, true},
			{"tiny-exponent", json.Number("1e-999999999999999999999999"), true, false},
			{"zero-exponent", json.Number("0e-999999999999999999999999"), true, true},
			{"string", "2", false, false},
			{"boolean", true, false, false},
			{"null", nil, false, false},
			{"object", map[string]any{"value": 2}, false, false},
			{"NaN", math.NaN(), false, false},
			{"infinity", math.Inf(1), false, false},
			{"invalid-JSON-number", json.Number("01"), false, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				declaration := capabilityDeclaration(t, "text", map[string]any{"weight": tc.value})
				reason := manifest.Check(declaration, []retrieval.DeclarationInput{{Type: "string"}})
				want := tc.number
				if kind == "integer" {
					want = tc.integer
				}
				if (reason == "") != want {
					t.Fatalf("published %s option judges %#v incorrectly: supported=%v reason=%s", kind, tc.value, want, reason)
				}
			})
		}
	}
}

func TestManifestNumericOptionEnumerationsUseExactNumericValues(t *testing.T) {
	for _, kind := range []string{"number", "integer"} {
		manifest := numericManifest(t, kind, []string{"9007199254740993", "2"})
		for _, tc := range []struct {
			name  string
			value any
			valid bool
		}{
			{"same-decimal-value", json.Number("2.0"), true},
			{"same-scientific-value", json.Number("9.007199254740993e15"), true},
			{"different-large-integer", json.Number("9007199254740992"), false},
			{"unlisted", 3, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				declaration := capabilityDeclaration(t, "text", map[string]any{"weight": tc.value})
				if reason := manifest.Check(declaration, []retrieval.DeclarationInput{{Type: "string"}}); (reason == "") != tc.valid {
					t.Fatalf("numeric enumeration judged %#v incorrectly: %s", tc.value, reason)
				}
			})
		}
	}
}
