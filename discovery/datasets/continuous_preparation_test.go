package datasets

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestContinuousPreparationFreezesAndChecksBeforeBuildingWithoutSeparateReacquisition(t *testing.T) {
	order := []string{}
	ticket := &trackedPublicationProtection{}
	definition := dataset.Dataset{SetID: "continuous", Revision: 3, Sources: []dataset.DatasetSource{{Repository: "a", Commit: "a1"}, {Repository: "b", Commit: "b1"}}}
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
		PrepareDataset: func(context.Context, dataset.ResolvedDataset) error {
			t.Fatal("separate build callback invoked")
			return nil
		},
		ProtectDataset: func(context.Context, dataset.ResolvedDataset) (PublicationProtection, error) {
			t.Fatal("ticket reacquired after build")
			return nil, nil
		},
		PrepareProtectedDataset: func(_ context.Context, candidate dataset.ResolvedDataset) (PublicationProtection, error) {
			order = append(order, "protected-build")
			if candidate.Dependencies["schema"] != "s1" || candidate.SetID != "continuous" || candidate.Revision != 3 {
				t.Fatal("protected build lost fixed dependencies")
			}
			return ticket, nil
		},
	}
	prepared, actual, err := p.ExecuteProtected(context.Background(), definition)
	if err != nil || actual != ticket || prepared.Dependencies["schema"] != "s1" || !reflect.DeepEqual(order, []string{"dependencies", "conflicts", "protected-build"}) {
		t.Fatalf("continuous preparation: %+v %v %v", prepared, order, err)
	}
	if ticket.cancellations != 0 || ticket.closes != 0 {
		t.Fatal("preparation released returned ticket")
	}
}
