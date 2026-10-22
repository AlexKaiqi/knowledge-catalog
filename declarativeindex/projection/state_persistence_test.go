package projection

import (
	"context"
	"errors"
	"os"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"

	bolt "go.etcd.io/bbolt"
)

func TestStateValuesRecoverOnlyAcceptedPublicationAfterLostAcknowledgment(t *testing.T) {
	for _, applied := range []bool{false, true} {
		name := "rejected"
		if applied {
			name = "accepted-with-lost-ack"
		}
		t.Run(name, func(t *testing.T) {
			repo, at, _ := testfixture.StateProjection(t)
			dir := t.TempDir()
			engine := &statePublicationFailureEngine{}
			open := func() *Index {
				idx := NewIndexEngine(dir, func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
				t.Cleanup(func() { _ = idx.Close() })
				return idx
			}
			idx := open()
			lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-20T00:00:00Z"}}
			original, err := idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: lookup})
			if err != nil {
				t.Fatal(err)
			}
			oldFile := idx.states[stateStoreKey(repo.ID(), at)].store.file
			lookup.Value = map[string]any{"status": "stopped"}
			lookup.Basis.SourceRevision = "r2"
			lookup.Basis.ObservedAt = "2026-10-20T00:01:00Z"
			fault := kernel.Fail(kernel.ErrTemporaryUnavailable, "publication acknowledgment unavailable")
			engine.commitErr, engine.commitApplied = fault, applied
			if _, err := idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: lookup}); !errors.Is(err, fault) {
				t.Fatalf("commit failure lost: %v", err)
			}
			candidate := engine.candidate.Revision
			candidateFile := observationFile(dir, stateStoreKey(repo.ID(), at), authorityEngineID(repo), candidate)
			if err := idx.Close(); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{oldFile, candidateFile} {
				if _, err := os.Stat(file); err != nil {
					t.Fatalf("publication values removed on Close: %s: %v", file, err)
				}
			}
			wantRevision, wantStatus, wantSource := original.Revision, "running", "r1"
			wantTime := "2026-10-20T00:00:00Z"
			if applied {
				wantRevision, wantStatus, wantSource = candidate, "stopped", "r2"
				wantTime = lookup.Basis.ObservedAt
			}
			calls := lookup.Calls
			for range 2 { // Two independent owners may read one immutable value file.
				reopened := open()
				if ready, err := reopened.StateProjectionReady(context.Background(), repo, at); err != nil || !ready {
					t.Fatalf("accepted values not recovered: %v %v", ready, err)
				}
				query := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: wantStatus})
				result, err := reopened.SearchStateAtRevision(repo, at, wantRevision, query)
				if err != nil || len(result.Hits) != 1 || result.Hits[0].Version.Observations[0].Basis.SourceRevision != wantSource {
					t.Fatalf("recovery mixed or refreshed observations: %#v %v", result, err)
				}
				if got := reopened.states[stateStoreKey(repo.ID(), at)].oldestObservedAt; got != wantTime {
					t.Fatalf("recovered age differs from fixed observations: got=%s want=%s", got, wantTime)
				}
				wrong := original.Revision
				if !applied {
					wrong = candidate
				}
				if _, err := reopened.SearchStateAtRevision(repo, at, wrong, query); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
					t.Fatalf("unaccepted or replaced revision served: %v", err)
				}
				if err := reopened.states[stateStoreKey(repo.ID(), at)].store.PutBatch(nil); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
					t.Fatalf("sealed values writable: %v", err)
				}
			}
			if lookup.Calls != calls {
				t.Fatal("recovery called external runtime")
			}
		})
	}
}

func TestStateValuesRecoveryRejectsCorruptionAndDoesNotRepairMissingMedia(t *testing.T) {
	for _, damage := range []string{"missing", "descriptor", "values"} {
		t.Run(damage, func(t *testing.T) {
			repo, at, _ := testfixture.StateProjection(t)
			dir := t.TempDir()
			engine := &testfixture.StateEngine{}
			open := func() *Index {
				return NewIndexEngine(dir, func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			}
			idx := open()
			lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-20T00:00:00Z"}}
			if _, err := idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: lookup}); err != nil {
				t.Fatal(err)
			}
			file := idx.states[stateStoreKey(repo.ID(), at)].store.file
			if err := idx.Close(); err != nil {
				t.Fatal(err)
			}
			if damage == "missing" {
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			} else {
				db, err := bolt.Open(file, 0600, nil)
				if err != nil {
					t.Fatal(err)
				}
				err = db.Update(func(tx *bolt.Tx) error {
					if damage == "descriptor" {
						return tx.DeleteBucket(stateDescriptorBucket)
					}
					return tx.Bucket(stateValueBucket).Put([]byte("object/corrupt"), []byte(`{}`))
				})
				_ = db.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			reopened := open()
			defer reopened.Close()
			ready, err := reopened.StateProjectionReady(context.Background(), repo, at)
			if ready || (damage != "missing" && kernel.CodeOf(err) != kernel.ErrPreconditionFailed) || (damage == "missing" && err != nil) {
				t.Fatalf("damaged media accepted: ready=%v err=%v", ready, err)
			}
			if damage == "missing" {
				if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("query recreated missing values: %v", err)
				}
			}
		})
	}
}

func TestStateValuesColdPhysicalIncompatibilityIsRepairableReadiness(t *testing.T) {
	repo, at, _ := testfixture.StateProjection(t)
	dir := t.TempDir()
	engine := &stateReadinessEngine{}
	open := func() *Index {
		return NewIndexEngine(dir, func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	}
	idx := open()
	lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-20T00:00:00Z"}}
	if _, err := idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: lookup}); err != nil {
		t.Fatal(err)
	}
	accepted := engine.Meta
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := open()
	defer reopened.Close()
	engine.Meta.PhysicalDigest = "old-physical-configuration"
	if ready, err := reopened.StateProjectionReady(context.Background(), repo, at); ready || err != nil {
		t.Fatalf("incompatible physical configuration is not repairable: %v %v", ready, err)
	}
	if len(reopened.states) != 0 {
		t.Fatal("incompatible observation adopted")
	}
	engine.Meta = accepted
	if ready, err := reopened.StateProjectionReady(context.Background(), repo, at); !ready || err != nil {
		t.Fatalf("valid observation could not recover after mismatch: %v %v", ready, err)
	}
	if lookup.Calls != 1 {
		t.Fatalf("readiness performed external reads: %d", lookup.Calls)
	}
}
