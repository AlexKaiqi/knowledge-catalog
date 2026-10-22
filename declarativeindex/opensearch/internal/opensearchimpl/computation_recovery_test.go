package opensearchimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
	ports "kc/discovery/projection"
	"kc/kernel"
)

func computationPartition(t *testing.T, engine *openSearchEngine, directory, model string, count int) computationCache {
	t.Helper()
	cache := engine.computationCache(directory)
	cache.identity = model
	cache.dimensions = 2
	keys := make([]string, count)
	vectors := map[string][]float32{}
	for i := range keys {
		keys[i] = fmt.Sprintf("value-%04d", i)
		vectors[keys[i]] = []float32{1, 2}
	}
	_, owners, err := cache.claim(keys)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.publish(owners, vectors); err != nil {
		t.Fatal(err)
	}
	return cache
}
func computationCoordinator(t *testing.T, engine *openSearchEngine) ports.ComputationMaintainer {
	t.Helper()
	coordinator, err := OpenResources(Config{URL: engine.base, Namespace: engine.namespace, PrimaryShards: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coordinator.Close() })
	return coordinator.(ports.ComputationMaintainer)
}
func TestComputationRecoverySweepsInactiveModelsAcrossColdCoordinatorReopen(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	var cache computationCache
	for i := 0; i < 40; i++ {
		cache = computationPartition(t, engine, directory, fmt.Sprintf("model-%03d", i), 1)
		expireComputationEntries(t, cache)
	}
	expected := []int{16, 16, 8}
	for _, count := range expected {
		report, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory)
		if err != nil || len(report.Deleted) != count || len(report.Retained) != 0 {
			t.Fatalf("cold model sweep lost progress: %+v %v", report, err)
		}
	}
	if err := cache.globalTransaction(func(models, meta *bolt.Bucket) error {
		key, _ := models.Cursor().First()
		if key != nil || cacheBytes(meta) != 0 {
			t.Fatalf("inactive models did not converge: %s bytes=%d", key, cacheBytes(meta))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestComputationRecoveryResumesDeepModelAndDoesNotResurrectExpiredOwner(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	cache := computationPartition(t, engine, directory, "old-model", 1000)
	expireComputationEntries(t, cache)
	coordinator := computationCoordinator(t, engine)
	report, err := coordinator.RecoverComputations(context.Background(), directory)
	if err != nil || !report.More || len(report.Deleted) != 0 {
		t.Fatalf("deep model was not bounded: %+v %v", report, err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		if records.Stats().KeyN != 744 {
			t.Fatalf("model sweep exceeded 256 values: %d", records.Stats().KeyN)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory); err != nil {
			t.Fatal(err)
		}
	}
	_, owners, err := cache.claim([]string{"staging"})
	if err != nil {
		t.Fatal(err)
	}
	report, err = coordinator.RecoverComputations(context.Background(), directory)
	if err != nil || len(report.Protected) != 1 {
		t.Fatalf("active computation was removed: %+v %v", report, err)
	}
	expireComputationEntries(t, cache)
	report, err = coordinator.RecoverComputations(context.Background(), directory)
	if err != nil || len(report.Deleted) != 1 {
		t.Fatalf("expired staging model remained: %+v %v", report, err)
	}
	if err := cache.publish(owners, map[string][]float32{"staging": {9, 9}}); err != nil {
		t.Fatal(err)
	}
	if err := cache.globalTransaction(func(models, meta *bolt.Bucket) error {
		key, _ := models.Cursor().First()
		if key != nil {
			t.Fatalf("late publication recreated swept model: %s", key)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestComputationRecoveryRetainsUnknownPartitionAndContinuesKnownModels(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	bad := computationPartition(t, engine, directory, "a-unknown", 2)
	good := computationPartition(t, engine, directory, "z-known", 1)
	expireComputationEntries(t, bad)
	expireComputationEntries(t, good)
	if err := bad.transaction(func(records, meta *bolt.Bucket) error {
		return records.Put([]byte("value-0001"), []byte(`{"version":999}`))
	}); err != nil {
		t.Fatal(err)
	}
	report, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory)
	if err == nil || len(report.Retained) != 1 || report.Retained[0] != "a-unknown" || len(report.Deleted) != 1 || report.Deleted[0] != "z-known" {
		t.Fatalf("unknown partition blocked independent cleanup: %+v %v", report, err)
	}
	if err := bad.transaction(func(records, meta *bolt.Bucket) error {
		if records.Stats().KeyN != 2 {
			t.Fatal("corrupt bounded batch was partially deleted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestComputationInstanceBudgetAppliesAcrossModelConfigurations(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	first := computationPartition(t, engine, directory, "first-model", 1)
	var before uint64
	if err := first.globalTransaction(func(models, meta *bolt.Bucket) error {
		before = cacheBytes(meta)
		return putCacheBytes(meta, computationCacheLimit)
	}); err != nil {
		t.Fatal(err)
	}
	second := engine.computationCache(directory)
	second.identity = "new-model"
	second.dimensions = 2
	hits, owners, err := second.claim([]string{"new-input"})
	if err != nil || len(hits) != 0 || len(owners) != 0 {
		t.Fatalf("new model exceeded instance budget: %v %v %v", hits, owners, err)
	}
	if err := first.globalTransaction(func(models, meta *bolt.Bucket) error {
		if models.Bucket([]byte("new-model")) != nil || cacheBytes(meta) != computationCacheLimit {
			t.Fatal("full instance created another model partition")
		}
		return putCacheBytes(meta, before)
	}); err != nil {
		t.Fatal(err)
	}
}
func TestComputationRecoveryCancellationDoesNotInitializeMissingRuntime(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir() + "/not-created"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := computationCoordinator(t, engine).RecoverComputations(ctx, directory); err != context.Canceled {
		t.Fatalf("cancelled recovery ignored context: %v", err)
	}
	report, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory)
	if err != nil || len(report.Deleted) != 0 || len(report.Retained) != 0 {
		t.Fatalf("absent optional cache was not empty: %+v %v", report, err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("maintenance initialized missing runtime: %v", err)
	}
}
func TestComputationPartitionDigestDetectsAlteredFiniteVector(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	cache := computationPartition(t, engine, directory, "model", 1)
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		entry, err := cache.decode([]byte("value-0000"), records.Get([]byte("value-0000")))
		if err != nil {
			return err
		}
		entry.Vector[0] = 99
		_, err = cache.put(records, meta, "value-0000", entry)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.claim([]string{"value-0000"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("altered finite vector was reused: %v", err)
	}
}

func TestComputationModelRotationReusesPhysicalPagesAfterIndependentSweep(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	var initial, maximum int64
	for cycle := 0; cycle < 8; cycle++ {
		cache := computationPartition(t, engine, directory, fmt.Sprintf("rotated-%d", cycle), 1000)
		expireComputationEntries(t, cache)
		for pass := 0; pass < 5; pass++ {
			if _, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory); err != nil {
				t.Fatal(err)
			}
		}
		if err := cache.globalTransaction(func(models, meta *bolt.Bucket) error {
			key, _ := models.Cursor().First()
			if key != nil || cacheBytes(meta) != 0 {
				t.Fatal("rotated model retained logical computation resources")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(cache.directory, "vectors.db"))
		if err != nil {
			t.Fatal(err)
		}
		if cycle == 0 {
			initial = info.Size()
		}
		if info.Size() > maximum {
			maximum = info.Size()
		}
	}
	if maximum > 4*initial {
		t.Fatalf("physical pages grow with model rotations: first=%d maximum=%d", initial, maximum)
	}
	t.Logf("eight cold model rotations: initial file=%d bytes, high-water=%d bytes, final logical records=0", initial, maximum)
}

func TestComputationRecoveryDoesNotTreatMissingExistingMediumAsEmpty(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	cache := computationPartition(t, engine, directory, "model", 1)
	path := filepath.Join(cache.directory, "vectors.db")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	report, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(report.Retained) != 1 || len(report.Deleted) != 0 {
		t.Fatalf("lost medium was treated as empty: %+v %v", report, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("maintenance recreated missing medium: %v", err)
	}
}
func TestComputationMediumReplacementCannotAcceptPreviousStagingOwner(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	cache := engine.computationCache(directory)
	cache.identity = "model"
	cache.dimensions = 2
	_, old, err := cache.claim([]string{"input"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cache.directory, "vectors.db")); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := cache.claim([]string{"input"})
	if err != nil || old["input"] == fresh["input"] {
		t.Fatalf("fresh computation reused previous medium owner: %v", err)
	}
	if err := cache.publish(old, map[string][]float32{"input": {99, 99}}); err != nil {
		t.Fatal(err)
	}
	hits, _, err := cache.claim([]string{"input"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("previous medium owner reappeared: %v %v", hits, err)
	}
	if err := cache.publish(fresh, map[string][]float32{"input": {1, 2}}); err != nil {
		t.Fatal(err)
	}
	hits, _, err = cache.claim([]string{"input"})
	if err != nil || len(hits) != 1 || hits["input"][0] != 1 {
		t.Fatalf("fresh computation could not publish: %v %v", hits, err)
	}
}

func TestComputationUnknownFieldsRemainQuarantinedWithoutPartialCleanup(t *testing.T) {
	engine := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned"}
	directory := t.TempDir()
	cache := computationPartition(t, engine, directory, "unknown-fields", 2)
	expireComputationEntries(t, cache)
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		raw := records.Get([]byte("value-0001"))
		altered := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"futureAcceptance":true}`)...)
		return records.Put([]byte("value-0001"), altered)
	}); err != nil {
		t.Fatal(err)
	}
	report, err := computationCoordinator(t, engine).RecoverComputations(context.Background(), directory)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(report.Retained) != 1 || len(report.Deleted) != 0 {
		t.Fatalf("future record was interpreted as current: %+v %v", report, err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		if records.Stats().KeyN != 2 {
			t.Fatal("unknown-field batch was partially deleted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestComputationRecoveryFixedViewRejectsMaintenanceBeforeFilesystemAccess(t *testing.T) {
	view := &openSearchEngine{base: "http://native.invalid:9200", namespace: "owned", fixedControl: []byte(`{"state":"READY"}`)}
	directory := t.TempDir() + "/not-created"
	report, err := view.RecoverComputations(context.Background(), directory)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(report.Deleted) != 0 || len(report.Protected) != 0 {
		t.Fatalf("fixed query view acquired maintenance authority: %+v %v", report, err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("fixed view accessed maintenance filesystem: %v", err)
	}
}
