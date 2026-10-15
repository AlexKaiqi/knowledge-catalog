package datasets

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
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

// Dataset publication refuses a candidate whose sources hold the same Aspect
// of one object, without any plugin configured, and keeps the serving version.
func TestDatasetPublicationRejectsAspectConflictAcrossSources(t *testing.T) {
	store := snapshot.NewRegistry()
	ids := []kernel.RepositoryID{}
	for _, id := range []string{"kr://publication/a", "kr://publication/b"} {
		s := testkit.NewSetup(t, id)
		if _, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
			Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "structure"}, Value: map[string]any{"from": id}}}}); err != nil {
			t.Fatal(err)
		}
		if err := store.Add(s.Repo); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.RepositoryID)
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("orders", 1, []dataset.DatasetSource{{Repository: ids[0], Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	objects := reader.NewReader(store)
	publisher := DatasetPublisher{Registry: cat,
		Authorize: func(context.Context, DatasetPublication) error { return nil },
		Prepare: Preparation{Conflicts: func(candidate dataset.ResolvedDataset) error {
			return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return objects.Require(id, kernel.ErrCapabilityUnsatisfied)
			}, testkit.DatasetPin(candidate)).CheckHoldingConflicts(nil)
		}}.Execute}
	_, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "orders", Revision: 2, Sources: []dataset.DatasetSource{
		{Repository: ids[0], Selector: snapshot.DefaultRef, Path: dataset.MountPath("a")},
		{Repository: ids[1], Selector: snapshot.DefaultRef, Path: dataset.MountPath("b")},
	}})
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || !strings.Contains(err.Error(), "table/orders") || !strings.Contains(err.Error(), "structure") {
		t.Fatalf("conflicting publication must be rejected with OBJECT_ID_CONFLICT naming object and Aspect, got %v", err)
	}
	if current, err := cat.Set("orders"); err != nil || current.Revision != 1 {
		t.Fatalf("rejected publication changed the serving version: %+v %v", current, err)
	}
	if err := (Preparation{}).Execute(context.Background(), dataset.Dataset{Sources: []dataset.DatasetSource{{Repository: ids[0]}, {Repository: ids[1]}}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("multi-source publication without a conflict check must not skip the conflict check, got %v", err)
	}
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
	latest, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: testkit.PutEntity("fixture/next", map[string]any{"body": "next"}, "")})
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
