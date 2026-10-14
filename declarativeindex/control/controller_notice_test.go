package control

import (
	"context"
	fixtures "kc/declarativeindex/internal/testfixture"
	query "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	sourcebridge "kc/liveaccess/discovery"
	"path/filepath"
	"strings"
	"testing"
	"time"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func dualLaneIndex(t *testing.T) *query.Index {
	t.Helper()
	snapshotEngine := &fixtures.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
	stateEngine := &fixtures.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
	idx := query.NewIndexEngine("", func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		if strings.Contains(string(id), "#state") {
			return stateEngine, nil
		}
		return snapshotEngine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestProjectionControllerNoticePullsStateWithoutChangingSnapshot(t *testing.T) {
	repo, commit, address := fixtures.StateProjection(t)
	idx := dualLaneIndex(t)
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
		func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if id != repo.ID() {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "not an object repository")
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

	lookup := &fixtures.StateLookup{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	controller.SetObservationSource(sourcebridge.Source{Lookup: lookup})
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(retrieval.SearchMATCH("running"))
	hits, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("cold-start State SEARCH: %#v %v", hits, err)
	}
	lookupsAfterSearch := lookup.Calls
	hits, err = idx.SearchStateAt(repo, commit, request)
	if err != nil || lookup.Calls != lookupsAfterSearch {
		t.Fatalf("SEARCH must not pull runtime: lookups=%d err=%v", lookup.Calls, err)
	}

	lookup.Value = map[string]any{"status": "stopped"}
	lookup.Basis.SourceRevision = "r2"
	hits, err = idx.SearchStateAt(repo, commit, request)
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("SEARCH without notice still used published observation: %#v %v", hits, err)
	}

	if err := controller.Notify(ports.ChangeNotice{Repository: repo.ID(), Address: &address, SourceRevision: "r2"}); err != nil {
		t.Fatal(err)
	}
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(stopped.Hits) != 1 {
		t.Fatalf("notice did not publish new observation: %#v %v", stopped, err)
	}
	raw, err := repo.Read(address.ObjectID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := raw.Value.(map[string]any); body["runtime"] != nil {
		t.Fatalf("notice wrote observation into Snapshot: %#v", raw)
	}
	if _, err := repo.ReadAddress(address, commit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("Bound State must not occupy a Snapshot unit: %#v %v", address, err)
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
	repo, commit, address := fixtures.StateProjection(t)
	idx := dualLaneIndex(t)
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
		func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if id != repo.ID() {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "not an object repository")
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
	lookup := &fixtures.StateLookup{
		Value: map[string]any{"status": "queued"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r0", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	controller.SetObservationSource(sourcebridge.Source{Lookup: lookup})
	t.Cleanup(controller.Close)
	controller.Start(context.Background())
	if err := controller.Notify(ports.ChangeNotice{Repository: repo.ID(), Address: &address}); err != nil {
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
	err = controller.Notify(ports.ChangeNotice{Repository: "kr://acme/public/core"})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("err=%v", err)
	}
}

func TestIngestionControllerKeepsStaticAspectAndDynamicRecipeLanesSeparate(t *testing.T) {
	repo, first, address := fixtures.StateProjection(t)
	idx := dualLaneIndex(t)
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
		func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if id != repo.ID() {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "not an object repository")
			}
			return repo, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	controller.SetInventory(func() ([]kernel.RepositoryID, error) {
		return []kernel.RepositoryID{repo.ID()}, nil
	})
	lookup := &fixtures.StateLookup{
		Value: map[string]any{"status": "running", "owner": "runtime"},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z",
		},
	}
	controller.SetObservationSource(sourcebridge.Source{Lookup: lookup})
	t.Cleanup(controller.Close)

	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	writable, ok := repo.(interface {
		ApplyObjectCommit(objectmodel.ChangeSet) (kernel.CommitID, error)
	})
	if !ok {
		t.Fatal("fixture repository is not writable")
	}
	second, err := writable.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first, ExpectedTargetCommit: first,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut,
			Address: objectmodel.Address{
				Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition",
			},
			Value: map[string]any{"owner": "platform"}, SchemaRef: "schema/job.definition",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// No event is required for correctness: reconciliation discovers the new
	// published HEAD and applies the static Aspect batch incrementally.
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err := controller.Targets()
	if err != nil || len(targets) != 1 || targets[0].AppliedCommit != second || targets[0].Status != ports.TargetReady {
		t.Fatalf("static lane did not reach the new Snapshot: %#v err=%v", targets, err)
	}
	staticQuery := retrieval.SearchOf(retrieval.SearchClause{
		Op: retrieval.OpEQ,
		Field: &retrieval.FieldRef{
			Schema: "schema/job.definition", Aspect: "definition", Path: "owner",
		},
		Value: "platform",
	})
	staticHits, err := idx.SearchAt(repo, second, staticQuery)
	if err != nil || len(staticHits.Hits) != 1 {
		t.Fatalf("static Aspect batch not searchable at %s: %#v err=%v", second, staticHits, err)
	}

	// A Recipe/Binding notice pulls a new observation without moving the
	// Snapshot basis or rewriting the static projection.
	lookup.Value = map[string]any{"status": "stopped", "owner": "runtime"}
	lookup.Basis.SourceRevision = "r2"
	lookup.Basis.ObservedAt = "2026-08-27T00:01:00Z"
	if err := controller.Notify(ports.ChangeNotice{
		Repository: repo.ID(), Address: &address, SourceRevision: "r2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	dynamicHits, err := idx.SearchStateAt(repo, second,
		retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(dynamicHits.Hits) != 1 {
		t.Fatalf("dynamic Recipe notice not searchable: %#v err=%v", dynamicHits, err)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head != second {
		t.Fatalf("dynamic lane moved Snapshot HEAD: head=%s err=%v", head, err)
	}
	meta, err := idx.Describe(repo)
	if err != nil || meta.BasisCommit != second {
		t.Fatalf("dynamic lane changed static projection Basis: %#v err=%v", meta, err)
	}
}
