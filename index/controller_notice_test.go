package index

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func dualLaneIndex(t *testing.T) *Index {
	t.Helper()
	snapshotEngine := &stateTestEngine{docs: map[knowledge.ObjectID]CompiledDoc{}}
	stateEngine := &stateTestEngine{docs: map[knowledge.ObjectID]CompiledDoc{}}
	idx := NewIndexEngine("", func(_ string, id kernel.RepositoryID) (Engine, error) {
		if strings.Contains(string(id), "#state") {
			return stateEngine, nil
		}
		return snapshotEngine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestProjectionControllerNoticePullsStateWithoutChangingSnapshot(t *testing.T) {
	repo, commit, address := stateProjectionFixture(t)
	idx := dualLaneIndex(t)
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
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

	lookup := &stateTestLookup{
		value: map[string]any{"status": "running"},
		basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	controller.SetStateLookup(lookup)
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(retrieval.SearchMATCH("running"))
	hits, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("cold-start State SEARCH: %#v %v", hits, err)
	}
	lookupsAfterSearch := lookup.n
	hits, err = idx.SearchStateAt(repo, commit, request)
	if err != nil || lookup.n != lookupsAfterSearch {
		t.Fatalf("SEARCH must not pull runtime: lookups=%d err=%v", lookup.n, err)
	}

	lookup.value = map[string]any{"status": "stopped"}
	lookup.basis.SourceRevision = "r2"
	hits, err = idx.SearchStateAt(repo, commit, request)
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("SEARCH without notice still used published observation: %#v %v", hits, err)
	}

	if err := controller.Notify(ChangeNotice{Repository: repo.ID(), Address: &address, SourceRevision: "r2"}); err != nil {
		t.Fatal(err)
	}
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(stopped.Hits) != 1 {
		t.Fatalf("notice did not publish new observation: %#v %v", stopped, err)
	}
	raw, err := repo.ReadAddress(address, commit)
	if err != nil || raw.Value != nil {
		t.Fatalf("notice wrote observation into Snapshot: %#v %v", raw, err)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head != commit {
		t.Fatalf("notice moved Repository HEAD: %s %v", head, err)
	}
	targets, err := controller.Targets()
	if err != nil || len(targets) != 1 || targets[0].AppliedCommit != commit {
		t.Fatalf("Snapshot Desire key was synthesized with State notice: %#v %v", targets, err)
	}
}

func TestProjectionControllerStartAppliesNotice(t *testing.T) {
	repo, commit, address := stateProjectionFixture(t)
	idx := dualLaneIndex(t)
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
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
	controller.SetReconcileInterval(20 * time.Millisecond)
	lookup := &stateTestLookup{
		value: map[string]any{"status": "queued"},
		basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationRepeatable, SourceRevision: "r0", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	controller.SetStateLookup(lookup)
	t.Cleanup(controller.Close)
	controller.Start(context.Background())
	if err := controller.Notify(ChangeNotice{Repository: repo.ID(), Address: &address}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		hits, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("queued")))
		if err == nil && len(hits.Hits) == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("Start worker did not apply change notice")
}

func TestChangeNoticeWithoutRuntimeIsCapabilityUnsatisfied(t *testing.T) {
	controller, err := NewController(nil, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = controller.Notify(ChangeNotice{Repository: "kr://acme/public/core"})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("err=%v", err)
	}
}
