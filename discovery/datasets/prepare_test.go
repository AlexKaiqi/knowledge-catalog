package datasets

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type datasetBasisRecorder struct {
	commits []kernel.CommitID
	err     error
}

func (r *datasetBasisRecorder) PrepareServingBasis(_ context.Context, _ objectmodel.Repository, at kernel.CommitID) error {
	r.commits = append(r.commits, at)
	return r.err
}

func TestDatasetRecoveryFollowsServingReleaseNotSourceHead(t *testing.T) {
	s := testkit.NewSetup(t, "")
	store := snapshot.NewRegistry()
	if err := store.Add(s.Repo); err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewService(store, testkit.MakeRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, cat, store)
	if _, err := cat.DefineDataset("active", 1, []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: testkit.PutEntity("next", map[string]any{"body": "next"}, "")})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &datasetBasisRecorder{err: errors.New("provider unavailable")}
	demand := ServingBasisDemand{Datasets: cat}
	assertDemand := func(want []kernel.CommitID) {
		t.Helper()
		bases, err := demand.Bases(s.RepositoryID)
		if err != nil || !reflect.DeepEqual(bases, want) {
			t.Fatalf("accepted demand: %v want=%v err=%v", bases, want, err)
		}
	}
	prepare := Preparation{Enabled: true, Provider: recorder, Lookup: func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }}
	assertDemand([]kernel.CommitID{s.RootCommitID})
	if err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err == nil {
		t.Fatal("readiness failure was discarded")
	}
	recorder.err = nil
	if err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commits) != 2 || recorder.commits[0] != s.RootCommitID || recorder.commits[1] != s.RootCommitID {
		t.Fatalf("recovered HEAD instead of serving release: %v", recorder.commits)
	}
	if _, err := cat.DefineDataset("active", 2, []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	assertDemand([]kernel.CommitID{latest})
	if err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err != nil {
		t.Fatal(err)
	}
	if recorder.commits[2] != latest {
		t.Fatalf("did not follow new accepted release: %v", recorder.commits)
	}
	if err := cat.RetireDataset("active"); err != nil {
		t.Fatal(err)
	}
	assertDemand([]kernel.CommitID{})
	if len(recorder.commits) != 3 {
		t.Fatal("retired release still scheduled")
	}
}
