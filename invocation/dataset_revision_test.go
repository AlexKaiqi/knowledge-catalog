package invocation

import (
	"testing"

	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestDatasetRevisionFlagAcceptsOnlyPositiveIntegers(t *testing.T) {
	for raw, want := range map[string]int{"": 0, "1": 1, " 12 ": 12} {
		got, err := DatasetRevisionFlag(map[string]invoke.FlagValue{RevisionFlag: raw})
		if err != nil || got != want {
			t.Fatalf("--revision %q = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"0", "-1", "two"} {
		if _, err := DatasetRevisionFlag(map[string]invoke.FlagValue{RevisionFlag: raw}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("--revision %q accepted: %v", raw, err)
		}
	}
}

func TestResolveAtRevisionRejectsAmbiguousScopesBeforeResolving(t *testing.T) {
	for name, test := range map[string]struct {
		setID string
		flags map[string]invoke.FlagValue
	}{
		"no named dataset":     {"", map[string]invoke.FlagValue{RevisionFlag: "1"}},
		"temporary definition": {"set", map[string]invoke.FlagValue{RevisionFlag: "1", WorkspaceDefinitionFlag: &dataset.Dataset{SetID: "set"}}},
		"explicit pin":         {"set", map[string]invoke.FlagValue{RevisionFlag: "1", "pin": `{"setId":"set"}`}},
	} {
		t.Run(name, func(t *testing.T) {
			// A nil service proves the request is refused before any lookup.
			if _, err := ResolveAtRevision(nil, "", nil, test.setID, test.flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("ambiguous revision scope accepted: %v", err)
			}
		})
	}
}
