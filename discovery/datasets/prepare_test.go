package datasets

import (
	"context"
	"errors"
	"fmt"
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
		Prepare: Preparation{Dependencies: noDependencies, Conflicts: func(candidate dataset.ResolvedDataset) error {
			return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return objects.Require(id, kernel.ErrCapabilityUnsatisfied)
			}, testkit.DatasetPin(candidate)).CheckHoldingConflicts()
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
	twoSources := dataset.Dataset{Sources: []dataset.DatasetSource{{Repository: ids[0]}, {Repository: ids[1]}}}
	if _, err := (Preparation{Dependencies: noDependencies}).Execute(context.Background(), twoSources); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "conflict check") {
		t.Fatalf("multi-source publication without a conflict check must not skip the conflict check, got %v", err)
	}
	if _, err := (Preparation{}).Execute(context.Background(), dataset.Dataset{Sources: []dataset.DatasetSource{{Repository: ids[0]}}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "dependency check") {
		t.Fatalf("publication without a dependency check must not skip reference dependencies, got %v", err)
	}
}

func noDependencies(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
	return nil, nil
}

func TestDatasetPreparationHandsOneFrozenCandidateToProvider(t *testing.T) {
	def := dataset.Dataset{SetID: "selected", Revision: 3,
		Sources: []dataset.DatasetSource{{Repository: "source", Commit: "fixed-source"}},
		Items:   []dataset.DatasetItem{{Repository: "source", Commit: "fixed-source", Prefix: "selected/"}},
	}
	dependencies := map[kernel.RepositoryID]kernel.CommitID{"dependency": "fixed-dependency"}
	refused := errors.New("corpus preparation refused")
	for _, failure := range []error{nil, refused} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			calls := 0
			legacy := &datasetBasisRecorder{}
			prepare := Preparation{Enabled: true, Provider: legacy,
				Lookup: func(kernel.RepositoryID) (objectmodel.Repository, error) {
					t.Fatal("whole-candidate preparation entered per-source lookup")
					return nil, nil
				},
				Dependencies: func(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
					return dependencies, nil
				},
				PrepareDataset: func(_ context.Context, candidate dataset.ResolvedDataset) error {
					calls++
					want := dataset.ResolvedDataset{SetID: def.SetID, Revision: def.Revision,
						Repositories: map[kernel.RepositoryID]kernel.CommitID{"source": "fixed-source"}, Items: def.Items, Dependencies: dependencies}
					if !reflect.DeepEqual(candidate, want) {
						t.Fatalf("provider lost frozen source, selection or dependency: got=%#v want=%#v", candidate, want)
					}
					return failure
				},
			}
			result, err := prepare.Execute(context.Background(), def)
			if !errors.Is(err, failure) || calls != 1 || len(legacy.commits) != 0 {
				t.Fatalf("whole-candidate preparation calls=%d legacy=%v result=%#v err=%v", calls, legacy.commits, result, err)
			}
			if failure == nil {
				want := def
				want.Dependencies = dependencies
				if !reflect.DeepEqual(result, want) {
					t.Fatalf("prepared candidate changed its frozen identity: got=%#v want=%#v", result, want)
				}
			} else if !reflect.DeepEqual(result, dataset.Dataset{}) {
				t.Fatalf("failed preparation returned an acceptable candidate: %#v", result)
			}
		})
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
	prepare := Preparation{Enabled: true, Provider: recorder, Dependencies: noDependencies, Lookup: func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }}
	assertDemand([]kernel.CommitID{s.RootCommitID})
	if _, err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err == nil {
		t.Fatal("readiness failure was discarded")
	}
	recorder.err = nil
	if _, err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commits) != 2 || recorder.commits[0] != s.RootCommitID || recorder.commits[1] != s.RootCommitID {
		t.Fatalf("recovered HEAD instead of serving release: %v", recorder.commits)
	}
	if _, err := cat.DefineDataset("active", 2, []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	assertDemand([]kernel.CommitID{latest})
	if _, err := prepare.Execute(context.Background(), cat.DumpState().Datasets[0]); err != nil {
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
