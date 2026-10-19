package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	sourcebridge "kc/liveaccess/discovery"
	"os"

	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"

	objectmodel "kc/object"
	"kc/snapshot"
)

func TestRequiresStateTreatsSystemRepositoryAsSnapshotOnly(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) {
		return nil, fmt.Errorf("unused")
	})
	t.Cleanup(func() { _ = idx.Close() })
	required, err := idx.RequiresState(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("merchandise")))
	if err != nil || required {
		t.Fatalf("System has no Binding declarations: required=%v err=%v", required, err)
	}
}

func TestStateRefreshFindsDynamicValueWithoutChangingSnapshot(t *testing.T) {
	repo, commit, address := testfixture.StateProjection(t)
	engine := &testfixture.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	lookup := &testfixture.StateLookup{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	request := retrieval.SearchOf(retrieval.SearchMATCH("running"))
	// Declarations reach the query side through a maintained projection.
	if _, err := idx.Ensure(context.Background(), repo, commit); err != nil {
		t.Fatal(err)
	}
	required, err := idx.RequiresState(repo, commit, request)
	if err != nil || !required {
		t.Fatalf("dynamic text query must select State projection: required=%v err=%v", required, err)
	}
	staticOnly := retrieval.SearchOf(retrieval.SearchClause{
		Op:    retrieval.OpEQ,
		Field: &retrieval.FieldRef{Schema: "schema/job.definition", Aspect: "definition", Path: "owner"},
		Value: "data",
	})
	if required, err := idx.RequiresState(repo, commit, staticOnly); err != nil || required {
		t.Fatalf("Snapshot-only query must not depend on State runtime: required=%v err=%v", required, err)
	}
	first, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil || first.Mode != ports.IndexModeRebuild || first.ObservationDigest == "" {
		t.Fatalf("first refresh: %#v %v", first, err)
	}
	result, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(result.Hits) != 1 || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("dynamic search: %#v %v", result, err)
	}
	value := result.Hits[0].Object.Value.(map[string]any)
	if value["runtime"].(map[string]any)["status"] != "running" {
		t.Fatalf("search hit did not hydrate from published Serving State: %#v", value)
	}
	raw, err := repo.Read(address.ObjectID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := raw.Value.(map[string]any); body["runtime"] != nil {
		t.Fatalf("State refresh must not write observation to Snapshot: %#v", raw)
	}
	if unit, err := repo.ReadAddress(address, commit); err != nil || unit.Value != nil || len(unit.Declarations) != 1 || !unit.Declarations[0].ValueSource.IsBinding() {
		t.Fatalf("Binding unit must stay a value-less declaration in Snapshot: %#v %v", unit, err)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head != commit {
		t.Fatalf("State refresh changed Repository HEAD: %s %v", head, err)
	}

	lookup.Value = map[string]any{"status": "stopped"}
	lookup.Basis.SourceRevision = "r2"
	lookup.Basis.ObservedAt = "2026-08-27T00:01:00Z"
	second, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil || second.Mode != ports.IndexModeRebuild || second.Revision == first.Revision {
		t.Fatalf("second bounded rebuild: %#v %v", second, err)
	}
	if _, err := idx.SearchStateAtRevision(repo, commit, first.Revision, request); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("stale outer SearchView must not read the new revision: %v", err)
	}
	oldResult, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(oldResult.Hits) != 0 {
		t.Fatalf("old dynamic value remained searchable: %#v %v", oldResult, err)
	}
	newResult, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(newResult.Hits) != 1 {
		t.Fatalf("new dynamic value not searchable: %#v %v", newResult, err)
	}
}

func TestStateRefreshObjectsOnlyTouchesNamedAddress(t *testing.T) {
	repo, commit, address := testfixture.StateProjection(t)
	engine := &testfixture.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	lookup := &testfixture.StateLookup{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	first, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	before := lookup.Calls
	lookup.Value = map[string]any{"status": "stopped"}
	lookup.Basis.SourceRevision = "r2"
	second, err := idx.RefreshStateObjects(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup}, []objectmodel.ObjectID{address.ObjectID})
	if err != nil || second.Mode != ports.IndexModeIncremental || second.Updated != 1 || second.Revision == first.Revision {
		t.Fatalf("incremental refresh: %#v %v", second, err)
	}
	if lookup.Calls != before+1 {
		t.Fatalf("incremental refresh enumerated the repository: lookups %d -> %d", before, lookup.Calls)
	}
	hits, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("stopped")))
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("incremental Value: %#v %v", hits, err)
	}
}

func TestStateServingStoreStreamsBoundedBatches(t *testing.T) {
	store, err := openStateServingStore(t.TempDir(), engineKey{repo: "kr://acme/public/scale", commit: "c1"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	file := store.file
	t.Cleanup(func() { _ = store.CloseAndRemove() })
	const total = 1201
	for start := 0; start < total; start += 137 {
		records := map[objectmodel.ObjectID]stateRecord{}
		for i := start; i < total && i < start+137; i++ {
			id := objectmodel.ObjectID(fmt.Sprintf("Item:%04d", i))
			records[id] = stateRecord{Doc: ports.CompiledDoc{ObjectID: id}}
		}
		if err := store.PutBatch(records); err != nil {
			t.Fatal(err)
		}
	}
	seen, batches, largest := 0, 0, 0
	if err := store.WalkDocs(500, func(docs []ports.CompiledDoc) error {
		batches++
		seen += len(docs)
		if len(docs) > largest {
			largest = len(docs)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != total || batches != 3 || largest != 500 {
		t.Fatalf("seen=%d batches=%d largest=%d", seen, batches, largest)
	}
	if err := store.CloseAndRemove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("Serving State file was not removed: %v", err)
	}
}

func TestStateServingStoreIdentityIsCollisionSafe(t *testing.T) {
	root := t.TempDir()
	left, err := openStateServingStore(root, engineKey{repo: "kr://acme/a/b", commit: "c1", lane: "state"}, "left")
	if err != nil {
		t.Fatal(err)
	}
	right, err := openStateServingStore(root, engineKey{repo: "kr://acme/a:b", commit: "c1", lane: "state"}, "right")
	if err != nil {
		_ = left.CloseAndRemove()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = left.CloseAndRemove()
		_ = right.CloseAndRemove()
	})
	if left.dir == right.dir {
		t.Fatalf("lossy path sanitization identified distinct State stores: %s", left.dir)
	}
}

func TestDynamicProjectionPublicEnvelopesStayCompact(t *testing.T) {
	for name, value := range map[string]any{
		"sync": ports.StateSync{
			Repository: "kr://acme/public/scale", BasisCommit: "c1", Revision: "r1",
			ObservationDigest: "digest", ObjectCount: 100_000_000,
		},
		"view": retrieval.SearchView{
			Snapshots:           map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/scale": "c1"},
			ProjectionRevisions: map[kernel.RepositoryID]string{"kr://acme/public/scale": "r1"},
		},
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "observations") || len(raw) > 512 {
			t.Fatalf("%s envelope grows with object cardinality: %s", name, raw)
		}
	}
}

func TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision(t *testing.T) {
	repo, commit, _ := testfixture.StateProjection(t)
	engine := &testfixture.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	lookup := &testfixture.StateLookup{
		Value: nil,
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly, SourceRevision: "r-null", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	first, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	missing := retrieval.SearchOf(retrieval.SearchClause{
		Op:    retrieval.OpMissing,
		Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "owner"},
	})
	result, err := idx.SearchStateAt(repo, commit, missing)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("observed null must be eligible for MISSING: %#v %v", result, err)
	}
	lookup.Err = errors.New("runtime unavailable")
	if _, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup}); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("failed lookup must fail refresh honestly: %v", err)
	}
	revision, ok := idx.StateView(repo.ID(), commit)
	if !ok || revision != first.Revision {
		t.Fatalf("failed refresh replaced published revision: %q want %q", revision, first.Revision)
	}
}
