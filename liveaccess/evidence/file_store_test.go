package evidence

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
)

func retainedSample(key string, value any) Sample {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: key}
	declaration := objectmodel.UnitDeclaration{Address: address, Digest: "unit-" + kernel.Digest(key), DeclarationDigest: "declaration-" + kernel.Digest(key), SchemaRef: "schema/service.health"}
	return Sample{Repository: "kr://scene/services", Commit: "fixed-1", Address: address, Declaration: declaration,
		Outcome: ValueOutcome, Value: value, Observation: &objectmodel.UnitObservation{
			Address: address, DeclarationCommit: "fixed-1", DeclarationDigest: declaration.DeclarationDigest,
			Basis: objectmodel.ObservationBasis{BindingGeneration: "source-v1", Consistency: objectmodel.ObservationLatestOnly,
				SourceRevision: "r1", ObservedAt: "2026-10-14T06:00:00Z"},
		}}
}

func retainedRecord() Record {
	failed := retainedSample("quota", nil)
	failed.Outcome, failed.Observation = FailureOutcome, nil
	failed.Failure = &Failure{Code: kernel.ErrTemporaryUnavailable, Message: "source Authorization verifier unavailable"}
	return Record{Identity: kernel.IdentityContext{Principal: "alice", OnBehalfOf: "operator"},
		Trace: kernel.TraceContext{TraceID: "trace-r1", SpanID: "span-r1"}, RequestID: "req-1",
		Units: []Sample{retainedSample("health", map[string]any{"status": "retained-first-source-value", "rows": json.Number("9007199254740993")}), retainedSample("optional", nil), failed}}
}

func retainedStore(t *testing.T, directory string) *FileStore {
	t.Helper()
	store, err := NewFileStore(directory, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store.MaxBytes = 1 << 20
	store.Now = func() time.Time { return time.Date(2026, 10, 14, 6, 1, 0, 123, time.UTC) }
	return store
}

func requireRecord(t *testing.T, store *FileStore, record Record) Reference {
	t.Helper()
	ref, err := store.Record(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func requireCode(t *testing.T, err error, code kernel.ErrorCode) {
	t.Helper()
	if kernel.CodeOf(err) != code {
		t.Fatalf("error %v has code %s; want %s", err, kernel.CodeOf(err), code)
	}
}

func TestRetainedResultsReopenPreservesOriginalValuesFailuresAndRequest(t *testing.T) {
	root := t.TempDir()
	store := retainedStore(t, root)
	original := retainedRecord()
	ref := requireRecord(t, store, original)
	if ref.Digest == "" || ref.OccurredAt == "" || ref.RetainUntil == "" {
		t.Fatalf("durable result returned an incomplete pointer: %+v", ref)
	}
	var pointer map[string]any
	raw, err := json.Marshal(ref)
	if err != nil || json.Unmarshal(raw, &pointer) != nil || len(pointer) != 4 {
		t.Fatalf("reference must contain only managed identity, digest and retention metadata: %s %v", raw, err)
	}
	originalDigest := kernel.CanonicalDigest(original)
	original.Units[0].Value.(map[string]any)["status"] = "latest-value"
	original.Units[2].Failure.Message = "latest-success"
	reopened := retainedStore(t, root)
	got, err := reopened.Read(context.Background(), ref)
	if err != nil || kernel.CanonicalDigest(got) != originalDigest {
		t.Fatalf("reopened result changed its actual value, failure, basis or request: %#v %v", got, err)
	}
	if got.Units[1].Value != nil || got.Units[1].Outcome != ValueOutcome || got.Units[1].Observation == nil {
		t.Fatalf("observed null became a failure or missing value: %+v", got.Units[1])
	}
	got.Units[0].Value.(map[string]any)["status"] = "caller-mutated"
	again, err := reopened.Read(context.Background(), ref)
	if err != nil || kernel.CanonicalDigest(again) != originalDigest {
		t.Fatalf("audit caller mutation changed the retained original: %#v %v", again, err)
	}
}

func TestRetainedValueFailureAndBasisLossFailWithinRetention(t *testing.T) {
	for _, component := range []string{"value", "failure", "basis", "record"} {
		t.Run(component, func(t *testing.T) {
			store := retainedStore(t, t.TempDir())
			ref := requireRecord(t, store, retainedRecord())
			path := store.path(ref.ID)
			if component == "record" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				removeStoredComponent(t, path, component)
			}
			_, err := store.Read(context.Background(), ref)
			requireCode(t, err, kernel.ErrTemporaryUnavailable)
		})
	}
}

func removeStoredComponent(t *testing.T, path, component string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	units := stored["record"].(map[string]any)["units"].([]any)
	switch component {
	case "value":
		delete(units[0].(map[string]any), "value")
	case "failure":
		delete(units[2].(map[string]any), "failure")
	case "basis":
		delete(units[0].(map[string]any)["observation"].(map[string]any), "basis")
	}
	raw, err = json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPruneRetainsPromisedResultsAndExpiresWithoutSourceFallback(t *testing.T) {
	store := retainedStore(t, t.TempDir())
	ref := requireRecord(t, store, retainedRecord())
	if err := store.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), ref); err != nil {
		t.Fatalf("prune deleted an unexpired promise: %v", err)
	}
	until, err := time.Parse(time.RFC3339Nano, ref.RetainUntil)
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return until }
	if err := store.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.path(ref.ID)); !os.IsNotExist(err) {
		t.Fatalf("expired retained record was not pruned: %v", err)
	}
	_, err = store.Read(context.Background(), ref)
	requireCode(t, err, kernel.ErrCapabilityUnsatisfied)
}

func TestCorruptPayloadCannotEndRetentionEarly(t *testing.T) {
	store := retainedStore(t, t.TempDir())
	ref := requireRecord(t, store, retainedRecord())
	path := store.path(ref.ID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	stored["retainUntil"] = "2026-10-13T00:00:00Z"
	raw, err = json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("corrupted deadline caused premature deletion: %v", err)
	}
	_, err = store.Read(context.Background(), ref)
	requireCode(t, err, kernel.ErrTemporaryUnavailable)
}

func TestRetainedWriteFailureCannotProduceAReceipt(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref, err := retainedStore(t, blocked).Record(context.Background(), retainedRecord())
	requireCode(t, err, kernel.ErrTemporaryUnavailable)
	if ref != (Reference{}) {
		t.Fatalf("failed durable write returned an apparently usable receipt: %+v", ref)
	}
}

func TestCapacityAndFloodFailuresKeepUnexpiredRecords(t *testing.T) {
	store := retainedStore(t, t.TempDir())
	ref := requireRecord(t, store, retainedRecord())
	info, err := os.Stat(store.path(ref.ID))
	if err != nil {
		t.Fatal(err)
	}
	store.MaxBytes = info.Size()
	failed, err := store.Record(context.Background(), retainedRecord())
	requireCode(t, err, kernel.ErrTemporaryUnavailable)
	if failed != (Reference{}) {
		t.Fatal("capacity failure produced a receipt")
	}
	store.MaxBytes = 1 << 20
	store.FloodStage, store.FloodUsedFraction = 0.95, func() float64 { return 0.96 }
	_, err = store.Record(context.Background(), retainedRecord())
	requireCode(t, err, kernel.ErrTemporaryUnavailable)
	if _, err := store.Read(context.Background(), ref); err != nil {
		t.Fatalf("capacity pressure destroyed a promised original: %v", err)
	}
}

func TestRecordReclaimsOnlyExpiredCapacity(t *testing.T) {
	store := retainedStore(t, t.TempDir())
	ref := requireRecord(t, store, retainedRecord())
	info, err := os.Stat(store.path(ref.ID))
	if err != nil {
		t.Fatal(err)
	}
	store.MaxBytes = info.Size()
	until, err := time.Parse(time.RFC3339Nano, ref.RetainUntil)
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return until }
	newRef := requireRecord(t, store, retainedRecord())
	if _, err := os.Stat(store.path(ref.ID)); !os.IsNotExist(err) {
		t.Fatalf("expired capacity was not reclaimed before new durable result: %v", err)
	}
	if _, err := store.Read(context.Background(), newRef); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentStoresEnforceOneSharedCapacity(t *testing.T) {
	root := t.TempDir()
	a, b := retainedStore(t, root), retainedStore(t, root)
	_, raw, _, err := a.prepare(retainedRecord())
	if err != nil {
		t.Fatal(err)
	}
	a.MaxBytes, b.MaxBytes = int64(len(raw)), int64(len(raw))
	var wg sync.WaitGroup
	refs := make([]Reference, 2)
	errs := make([]error, 2)
	for i, store := range []*FileStore{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refs[i], errs[i] = store.Record(context.Background(), retainedRecord())
		}()
	}
	wg.Wait()
	accepted := 0
	for i, err := range errs {
		if err == nil {
			accepted++
			if _, err := b.Read(context.Background(), refs[i]); err != nil {
				t.Fatal(err)
			}
		} else {
			requireCode(t, err, kernel.ErrTemporaryUnavailable)
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent quota check accepted %d records into one-record capacity", accepted)
	}
}
