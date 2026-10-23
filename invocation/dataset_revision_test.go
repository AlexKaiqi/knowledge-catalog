package invocation

import (
	"context"
	"errors"
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

// Default corpus consumption resolves to the newest ACCEPTED revision that is
// also READY: a newer accepted version whose corpus is still building must not
// hide the older one that can still be searched.
func TestNewestReadyRevisionSkipsAcceptedButNotReadyVersions(t *testing.T) {
	ready := map[int]bool{3: false, 2: true, 1: true}
	var probed []int
	revision, ok, err := newestReadyRevision(context.Background(), []int{5, 3, 2, 1}, func(revision int) (bool, error) {
		probed = append(probed, revision)
		if revision == 5 {
			// An accepted revision that cannot even be resolved is skipped, not
			// turned into a failure.
			return false, nil
		}
		return ready[revision], nil
	})
	if err != nil || !ok || revision != 2 {
		t.Fatalf("newest ready revision: %d %t %v", revision, ok, err)
	}
	if want := []int{5, 3, 2}; len(probed) != len(want) || probed[0] != want[0] || probed[1] != want[1] || probed[2] != want[2] {
		t.Fatalf("probe order must be newest-first and stop at the first ready: %v", probed)
	}
}

// When nothing is ready the answer is "no revision", never a guess: the caller
// keeps its ordinary not-prepared response instead of serving an older version
// that also cannot be searched.
func TestNewestReadyRevisionReportsNothingReady(t *testing.T) {
	revision, ok, err := newestReadyRevision(context.Background(), []int{2, 1}, func(int) (bool, error) { return false, nil })
	if err != nil || ok || revision != 0 {
		t.Fatalf("no ready revision: %d %t %v", revision, ok, err)
	}
	revision, ok, err = newestReadyRevision(context.Background(), nil, func(int) (bool, error) { return true, nil })
	if err != nil || ok || revision != 0 {
		t.Fatalf("no accepted revision: %d %t %v", revision, ok, err)
	}
}

func TestNewestReadyRevisionStopsOnReadinessFailureOrCancellation(t *testing.T) {
	problem := errors.New("readiness unavailable")
	if _, _, err := newestReadyRevision(context.Background(), []int{2, 1}, func(int) (bool, error) { return false, problem }); !errors.Is(err, problem) {
		t.Fatalf("readiness failure was swallowed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := newestReadyRevision(ctx, []int{1}, func(int) (bool, error) { return true, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was ignored: %v", err)
	}
}

// A default consumer selects a revision; an explicit request always wins and is
// never replaced by the convenience default.
func TestReadyRevisionTargetNeverOverridesAnExplicitScope(t *testing.T) {
	for name, test := range map[string]struct {
		flags map[string]invoke.FlagValue
		want  string
	}{
		"named dataset":        {map[string]invoke.FlagValue{"dataset": "articles"}, "articles"},
		"explicit revision":    {map[string]invoke.FlagValue{"dataset": "articles", RevisionFlag: "3"}, ""},
		"explicit pin":         {map[string]invoke.FlagValue{"dataset": "articles", "pin": `{"setId":"articles"}`}, ""},
		"temporary definition": {map[string]invoke.FlagValue{"dataset": "articles", WorkspaceDefinitionFlag: &dataset.Dataset{SetID: "articles"}}, ""},
		"no dataset":           {map[string]invoke.FlagValue{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := readyRevisionTarget(test.flags); err != nil || got != test.want {
				t.Fatalf("ready revision target = %q, %v; want %q", got, err, test.want)
			}
		})
	}
	if _, err := readyRevisionTarget(map[string]invoke.FlagValue{"dataset": "articles", RevisionFlag: "0"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid explicit revision was silently treated as default: %v", err)
	}
}
