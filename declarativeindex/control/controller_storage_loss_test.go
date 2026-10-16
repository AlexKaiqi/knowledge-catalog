package control

import (
	"context"
	fixtures "kc/declarativeindex/internal/testfixture"
	query "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Goal: index-control U3 must recover discarded physical state even when the
// durable work ledger is READY. Non-goals: provider availability, Dataset and
// State recovery. The real Writer, query.Index and Controller own those boundaries;
// the existing conformance engine supplies only discardable physical storage.
func TestProjectionControllerRecoversPhysicalLossWithReadyLedger(t *testing.T) {
	for _, mode := range []string{"catch_up_after_restart", "periodic_after_restart"} {
		t.Run(mode, func(t *testing.T) {
			repo := testkit.MakeRepository(t, "kr://acme/public/storage-loss")
			root := testkit.MustHead(t, repo, snapshot.DefaultRef)
			head, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
				TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
				BaseCommit: root, ExpectedTargetCommit: root,
				Operations: []objectmodel.Operation{
					{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
						"entity": "Policy", "pattern": "record",
						"fields": map[string]any{"body": map[string]any{}},
					}},
					testkit.IndexText("schema/policy.body", "body"),
					testkit.PutEntity("policy/P-1", map[string]any{"body": "tested runbook"}, "")[0],
					testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, "")[0],
					testkit.PutEntity("policy/unrelated", map[string]any{"body": "ordinary notes"}, "")[0],
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			ledger := filepath.Join(t.TempDir(), "controller.db")
			engine := &storageLossEngine{checked: make(chan struct{}, 1)}
			idx := storageLossIndex(t, engine)
			controller := storageLossController(t, idx, ledger, repo)
			if err := controller.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertStorageLossReadyLedger(t, controller, repo.ID(), head)

			// An independent full rebuild is the reference, including the unrelated
			// candidate that the real query.Index must eliminate by equivalent residual.
			full := storageLossIndex(t, &storageLossEngine{})
			if _, err := full.Rebuild(repo, head); err != nil {
				t.Fatal(err)
			}
			want := storageLossSearch(t, full, repo, head)
			if len(want.Hits) != 2 {
				t.Fatalf("full rebuild returned %d matches, want 2", len(want.Hits))
			}
			assertStorageLossSearchEqual(t, storageLossSearch(t, idx, repo, head), want)
			controller.Close()
			if err := idx.Close(); err != nil {
				t.Fatal(err)
			}

			if mode == "catch_up_after_restart" {
				engine.discard()
			}
			reopened := storageLossIndex(t, engine)
			recovery := storageLossController(t, reopened, ledger, repo)
			assertStorageLossReadyLedger(t, recovery, repo.ID(), head)
			if mode == "catch_up_after_restart" {
				desc, err := reopened.Describe(repo)
				if err != nil || desc.State == ports.ProjectionStateReady || desc.BasisCommit != "" || desc.ObjectCount != 0 {
					t.Fatalf("fault injection did not discard physical projection: %#v err=%v", desc, err)
				}
			}
			if mode == "periodic_after_restart" {
				// Observe the startup check, then serialize with it through the public
				// management port. Loss after this point requires another worker pass.
				select {
				case <-engine.checked:
				default:
				}
				recovery.SetReconcileInterval(20 * time.Millisecond)
				recovery.Start(context.Background())
				select {
				case <-engine.checked:
				case <-time.After(5 * time.Second):
					t.Fatal("restarted worker did not inspect physical state")
				}
				if err := recovery.CatchUp(context.Background()); err != nil {
					t.Fatal(err)
				}
				engine.discard()
			}

			// Nothing is republished, desired or notified after storage loss. The
			// completed durable record alone cannot stand in for a physical index.
			if mode == "catch_up_after_restart" {
				if err := recovery.CatchUp(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				deadline := time.Now().Add(5 * time.Second)
				for !engine.readyAt(head) && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				recovery.Close()
			}
			assertStorageLossReadyLedger(t, recovery, repo.ID(), head)
			desc, err := reopened.Describe(repo)
			// Three policies plus index/fixture, the ordinary object holding the
			// index declaration member.
			if err != nil || desc.State != ports.ProjectionStateReady || desc.BasisCommit != head || desc.ObjectCount != 4 {
				t.Fatalf("READY ledger did not restore physical projection: %#v err=%v", desc, err)
			}
			assertStorageLossSearchEqual(t, storageLossSearch(t, reopened, repo, head), want)
			if current := testkit.MustHead(t, repo, snapshot.DefaultRef); current != head {
				t.Fatalf("projection recovery changed object HEAD: got %s want %s", current, head)
			}
		})
	}
}

func storageLossIndex(t *testing.T, engine *storageLossEngine) *query.Index {
	t.Helper()
	idx := query.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func storageLossController(t *testing.T, idx *query.Index, path string, repo objectmodel.Repository) *Controller {
	t.Helper()
	controller, err := NewController(idx, NewTargetStore(path), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unknown repository")
		}
		return repo, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
	t.Cleanup(controller.Close)
	return controller
}

func assertStorageLossReadyLedger(t *testing.T, controller *Controller, repository kernel.RepositoryID, head kernel.CommitID) {
	t.Helper()
	targets, err := controller.Targets()
	if err != nil || len(targets) != 1 || targets[0].Repository != repository || targets[0].Status != ports.TargetReady || targets[0].DesiredCommit != head || targets[0].AppliedCommit != head {
		t.Fatalf("durable READY ledger: %#v err=%v", targets, err)
	}
}

func storageLossSearch(t *testing.T, idx *query.Index, repo objectmodel.Repository, head kernel.CommitID) retrieval.SearchResult {
	t.Helper()
	result, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || result.Completeness != retrieval.CompletenessComplete || result.Continuation != "" {
		t.Fatalf("complete fixed-basis search: %#v err=%v", result, err)
	}
	for _, hit := range result.Hits {
		canonical, err := repo.Read(hit.Object.Address.ObjectID, head)
		if err != nil || !reflect.DeepEqual(hit.Object, canonical) || !reflect.DeepEqual(hit.Version, retrieval.VersionOf(canonical)) {
			t.Fatalf("hit differs from complete object and version at %s: %#v err=%v", head, hit, err)
		}
	}
	return result
}

func assertStorageLossSearchEqual(t *testing.T, got, want retrieval.SearchResult) {
	t.Helper()
	if !reflect.DeepEqual(got.Hits, want.Hits) || !reflect.DeepEqual(got.SearchView.Snapshots, want.SearchView.Snapshots) || !reflect.DeepEqual(got.Claims, want.Claims) {
		t.Fatalf("recovered query differs from full rebuild: got=%#v want=%#v", got, want)
	}
}

// The existing fixture keeps real compiled documents and performs an exhaustive
// superset search; only locking and loss injection are added here.
type storageLossEngine struct {
	mu      sync.Mutex
	backend fixtures.StateEngine
	checked chan struct{}
}

func (e *storageLossEngine) Probe(clause retrieval.SearchClause, spec retrieval.AccessSpec) ports.Capability {
	return e.backend.Probe(clause, spec)
}
func (e *storageLossEngine) Retrieve(request ports.RetrieveRequest) (ports.CandidatePage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backend.Retrieve(request)
}
func (e *storageLossEngine) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	return e.backend.VerifyResidualMatch(text, query, mode)
}
func (e *storageLossEngine) LoadMeta() (ports.Meta, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case e.checked <- struct{}{}:
	default:
	}
	return e.backend.LoadMeta()
}
func (e *storageLossEngine) Rebuild(docs []ports.CompiledDoc, meta ports.Meta) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backend.Rebuild(docs, meta)
}
func (e *storageLossEngine) Apply(upserts []ports.CompiledDoc, deletes []objectmodel.ObjectID, meta ports.Meta) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backend.Apply(upserts, deletes, meta)
}
func (e *storageLossEngine) Count() (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backend.Count()
}
func (*storageLossEngine) Close() error { return nil }
func (e *storageLossEngine) discard() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.backend = fixtures.StateEngine{}
}
func (e *storageLossEngine) readyAt(commit kernel.CommitID) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backend.Meta.Basis == commit && e.backend.Meta.State == ports.ProjectionStateReady && len(e.backend.Docs) == 4
}
