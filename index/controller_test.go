package index_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func TestProjectionControllerDesireIsDurableAndCoalescesWithoutHydration(t *testing.T) {
	lookups := 0
	path := filepath.Join(t.TempDir(), "controller.db")
	store := index.NewTargetStore(path)
	controller, err := index.NewController(nil, store, func(kernel.RepositoryID) (knowledge.Repository, error) {
		lookups++
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Desire("kr://scale/physical", "commit-1"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Desire("kr://scale/physical", "commit-2"); err != nil {
		t.Fatal(err)
	}
	if lookups != 0 {
		t.Fatalf("receipt-path desire hydrated %d repositories", lookups)
	}
	targets, err := controller.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].DesiredCommit != "commit-2" || targets[0].Status != index.TargetPending {
		t.Fatalf("targets = %#v", targets)
	}

	reopened, err := index.NewController(nil, index.NewTargetStore(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	targets, err = reopened.Targets()
	if err != nil || len(targets) != 1 || targets[0].DesiredCommit != "commit-2" {
		t.Fatalf("reopened targets = %#v, %v", targets, err)
	}
}

func TestProjectionReconcileRecoversMissingDesire(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	idx := liveIndex(t)
	controller := newTestController(t, idx, repo)
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertSearchHits(t, idx, repo, head, "runbook", 1)
	assertLiveReadyAt(t, idx, repo, head)
}

func TestProjectionStartRecoversWithoutDesire(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	idx := liveIndex(t)
	controller := newTestController(t, idx, repo)
	controller.SetReconcileInterval(20 * time.Millisecond)
	controller.Start(context.Background())
	waitSearchHits(t, idx, repo, head, "runbook", 1)
	assertLiveReadyAt(t, idx, repo, head)
}

func TestProjectionCatchUpDoesNotSkipWhenHeadMoved(t *testing.T) {
	repo, root, c1 := committedSearchablePolicy(t)
	idx := liveIndex(t)
	controller := newTestController(t, idx, repo)
	if err := controller.Desire(repo.ID(), c1); err != nil {
		t.Fatal(err)
	}
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err := controller.Targets()
	if err != nil || len(targets) != 1 || targets[0].Status != index.TargetReady || targets[0].AppliedCommit != c1 {
		t.Fatalf("seeded ledger %#v %v", targets, err)
	}

	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "second runbook"}, ""))
	if c2 == c1 || c2 == root {
		t.Fatalf("HEAD did not move: %s", c2)
	}
	targets, err = controller.Targets()
	if err != nil || targets[0].DesiredCommit != c1 || targets[0].AppliedCommit != c1 || targets[0].Status != index.TargetReady {
		t.Fatalf("lost-notify ledger should still look ready at c1: %#v %v", targets, err)
	}

	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertSearchHits(t, idx, repo, c2, "runbook", 2)
	assertLiveReadyAt(t, idx, repo, c2)
	targets, err = controller.Targets()
	if err != nil || targets[0].DesiredCommit != c2 || targets[0].AppliedCommit != c2 || targets[0].Status != index.TargetReady {
		t.Fatalf("ledger after catch-up %#v %v", targets, err)
	}
}

func TestProjectionUnappliedHeadIsNotSearchableUntilCatchUp(t *testing.T) {
	repo, _, c1 := committedSearchablePolicy(t)
	idx := liveIndex(t)
	controller := newTestController(t, idx, repo)
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertSearchHits(t, idx, repo, c1, "runbook", 1)

	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "second runbook"}, ""))
	assertSearchHits(t, idx, repo, c1, "runbook", 1)
	_, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied && kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("new HEAD must fail closed before catch-up: %v", err)
	}

	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertSearchHits(t, idx, repo, c2, "runbook", 2)
}

func committedSearchablePolicy(t *testing.T) (*testkit.KnowledgeRepository, kernel.CommitID, kernel.CommitID) {
	t.Helper()
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := mustRepoHead(t, repo)
	head := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	return repo, root, head
}

func newTestController(t *testing.T, idx *index.Index, repo knowledge.Repository) *index.Controller {
	t.Helper()
	controller, err := index.NewController(idx, index.NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
		func(id kernel.RepositoryID) (knowledge.Repository, error) {
			if id != repo.ID() {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "not a knowledge repository")
			}
			return repo, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	controller.SetInventory(func() ([]kernel.RepositoryID, error) {
		return []kernel.RepositoryID{repo.ID()}, nil
	})
	t.Cleanup(controller.Close)
	return controller
}

func assertLiveReadyAt(t *testing.T, idx *index.Index, repo knowledge.Repository, head kernel.CommitID) {
	t.Helper()
	desc, err := idx.Describe(repo)
	if err != nil || desc.BasisCommit != head || desc.State != index.ProjectionStateReady || desc.LagBehindHead {
		t.Fatalf("live projection %#v %v", desc, err)
	}
}

func assertSearchHits(t *testing.T, idx *index.Index, repo knowledge.Repository, commit kernel.CommitID, query string, want int) {
	t.Helper()
	hits, err := idx.SearchAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH(query)))
	if err != nil || len(hits.Hits) != want {
		t.Fatalf("search %q: hits=%d want=%d err=%v", query, want, len(hits.Hits), err)
	}
}

func waitSearchHits(t *testing.T, idx *index.Index, repo knowledge.Repository, commit kernel.CommitID, query string, want int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last error
	n := 0
	for time.Now().Before(deadline) {
		hits, err := idx.SearchAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH(query)))
		last = err
		if err == nil {
			n = len(hits.Hits)
			if n == want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("search %q did not reach %d hits: last=%d err=%v", query, want, n, last)
}

func mustRepoHead(t *testing.T, repo knowledge.Repository) kernel.CommitID {
	t.Helper()
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	return head
}
