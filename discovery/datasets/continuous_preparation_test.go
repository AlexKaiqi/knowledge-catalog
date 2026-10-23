package datasets

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// Validation and build are two separate halves now. Validation freezes the
// dependency set and the conflict answer on the candidate; the build reads that
// frozen candidate afterwards and may not re-freeze it. Both halves see the
// same coordinates, which is what keeps a background build from drifting onto
// a newer source version between acceptance and build.
func TestValidationAndBuildShareOneFrozenCandidate(t *testing.T) {
	order := []string{}
	definition := dataset.Dataset{SetID: "decoupled", Revision: 3,
		Sources: []dataset.DatasetSource{{Repository: "a", Commit: "a1"}, {Repository: "b", Commit: "b1"}}}
	var built dataset.ResolvedDataset
	p := Preparation{Enabled: true,
		Dependencies: func(candidate dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
			order = append(order, "dependencies")
			return map[kernel.RepositoryID]kernel.CommitID{"schema": "s1"}, nil
		},
		Conflicts: func(candidate dataset.ResolvedDataset) error {
			order = append(order, "conflicts")
			if candidate.Repositories["a"] != "a1" || candidate.Repositories["b"] != "b1" {
				t.Fatal("candidate drifted")
			}
			return nil
		},
		PrepareDataset: func(_ context.Context, candidate dataset.ResolvedDataset) error {
			order = append(order, "build")
			built = candidate
			return nil
		},
	}
	prepared, err := p.Execute(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"dependencies", "conflicts"}) {
		t.Fatalf("validation built or skipped a check: %v", order)
	}
	if prepared.Dependencies["schema"] != "s1" {
		t.Fatalf("validation dropped frozen dependencies: %#v", prepared)
	}
	if err := p.Build(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	want := dataset.ResolvedDataset{SetID: "decoupled", Revision: 3,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{"a": "a1", "b": "b1"}, Items: prepared.Items,
		Dependencies: map[kernel.RepositoryID]kernel.CommitID{"schema": "s1"}}
	if !reflect.DeepEqual(built, want) {
		t.Fatalf("build lost the frozen candidate: got=%#v want=%#v", built, want)
	}
	if !reflect.DeepEqual(order, []string{"dependencies", "conflicts", "build"}) {
		t.Fatalf("build did not follow validation: %v", order)
	}
}

// A build for an unconfigured deployment has nothing to do and must still
// return the caller's context error rather than pretend it built something.
func TestBuildWithoutConfiguredProviderIsANoOp(t *testing.T) {
	if err := (Preparation{}).Build(context.Background(), dataset.Dataset{SetID: "plain", Revision: 1}); err != nil {
		t.Fatalf("plain Dataset build: %v", err)
	}
}
