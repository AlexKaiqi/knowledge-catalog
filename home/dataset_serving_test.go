package home

import (
	"context"
	"errors"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

type datasetBasisRecorder struct {
	commits []kernel.CommitID
	err     error
}

func (r *datasetBasisRecorder) PrepareServingBasis(_ context.Context, _ knowledge.Repository, at kernel.CommitID) error {
	r.commits = append(r.commits, at)
	return r.err
}

func TestDatasetRecoveryFollowsServingReleaseNotSourceHead(t *testing.T) {
	s := testkit.NewSetup(t, "")
	store := snapshot.NewRegistry()
	if err := store.Add(s.Repo); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, testkit.MakeRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, cat, store)
	if _, err := cat.DefineKnowledgeSet("active", 1, []catalog.KnowledgeSetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: testkit.PutEntity("next", map[string]any{"body": "next"}, "")})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &datasetBasisRecorder{err: errors.New("provider unavailable")}
	consumer := datasetProjectionConsumer{catalogs: []*catalog.Catalog{cat}, controller: recorder}
	if err := consumer.Reconcile(context.Background(), s.Repo, latest); err == nil {
		t.Fatal("readiness failure was discarded")
	}
	recorder.err = nil
	if err := consumer.Reconcile(context.Background(), s.Repo, latest); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commits) != 2 || recorder.commits[0] != s.RootCommitID || recorder.commits[1] != s.RootCommitID {
		t.Fatalf("recovered HEAD instead of serving release: %v", recorder.commits)
	}
	if _, err := cat.DefineKnowledgeSet("active", 2, []catalog.KnowledgeSetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Reconcile(context.Background(), s.Repo, latest); err != nil {
		t.Fatal(err)
	}
	if recorder.commits[2] != latest {
		t.Fatalf("did not follow new accepted release: %v", recorder.commits)
	}
	if err := cat.RetireKnowledgeSet("active"); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Reconcile(context.Background(), s.Repo, latest); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commits) != 3 {
		t.Fatal("retired release still scheduled")
	}
}
