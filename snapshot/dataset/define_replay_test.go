package dataset

import (
	"testing"

	"kc/kernel"
)

type versionOnly struct {
	Service
	accepted Dataset
}

func (v versionOnly) DatasetVersion(setID string, revision int) (Dataset, error) {
	if setID != v.accepted.SetID || revision != v.accepted.Revision {
		return Dataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "not published")
	}
	return v.accepted, nil
}

// A retry of the accepted definition replays it; frozen commits are what
// publication added, not part of the request. Any other difference falls
// through to the monotonic revision check.
func TestReplayedDefinitionMatchesOnlyTheSameRequest(t *testing.T) {
	requested := []DatasetSource{{Repository: "kr://a", Selector: "refs/heads/main", Path: MountPath("notes"), SubPath: "note"}}
	frozen := []DatasetSource{{Repository: "kr://a", Selector: "refs/heads/main", Path: MountPath("notes"), SubPath: "note", Commit: "c1"}}
	svc := versionOnly{accepted: Dataset{SetID: "s", Revision: 1, Sources: frozen, Metadata: map[string]string{"team": "ops"}}}
	if got, ok := ReplayedDefinition(svc, "s", 1, requested, map[string]string{"team": "ops"}); !ok || got.Sources[0].Commit != "c1" {
		t.Fatalf("same request must replay the accepted revision: %#v %v", got, ok)
	}
	explicit := append([]DatasetSource(nil), requested...)
	explicit[0].Commit = "c1"
	if _, ok := ReplayedDefinition(svc, "s", 1, explicit, map[string]string{"team": "ops"}); !ok {
		t.Fatal("the accepted explicit commit must replay")
	}
	for name, try := range map[string]struct {
		revision int
		sources  []DatasetSource
		metadata map[string]string
	}{
		"other revision": {2, requested, map[string]string{"team": "ops"}},
		"other path":     {1, []DatasetSource{{Repository: "kr://a", Selector: "refs/heads/main", Path: MountPath("moved"), SubPath: "note"}}, map[string]string{"team": "ops"}},
		"other metadata": {1, requested, map[string]string{"team": "sre"}},
		"no metadata":    {1, requested, nil},
		"pinned commit":  {1, []DatasetSource{{Repository: "kr://a", Selector: "refs/heads/main", Path: MountPath("notes"), SubPath: "note", BaseRev: "c0"}}, map[string]string{"team": "ops"}},
		"other commit":   {1, []DatasetSource{{Repository: "kr://a", Selector: "refs/heads/main", Path: MountPath("notes"), SubPath: "note", Commit: "c2"}}, map[string]string{"team": "ops"}},
	} {
		if _, ok := ReplayedDefinition(svc, "s", try.revision, try.sources, try.metadata); ok {
			t.Fatalf("%s must not replay", name)
		}
	}
}
