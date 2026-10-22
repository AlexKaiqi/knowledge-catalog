package opensearchimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
	ports "kc/discovery/projection"
	"kc/kernel"
)

func memberComputationEngine(directory string) *openSearchEngine {
	return &openSearchEngine{namespace: "owned", computationCacheDirectory: directory, computationCacheAuthority: "test-instance-v2", embeddingCacheIdentity: "test-vector-model", vectorDimension: 2, vectorModel: "test@1", embedder: &countedEmbedding{model: "test@1"}}
}
func memberValueCache(e *openSearchEngine) computationCache {
	return computationCache{directory: e.computationCacheDirectory, authority: e.computationCacheAuthority, identity: "member-values-v1", memberValues: true}
}

func TestMemberComputationPersistsWithIndependentVectorPartition(t *testing.T) {
	directory := t.TempDir()
	first := memberComputationEngine(directory)
	calls := 0
	key := kernel.CanonicalDigest("member-value-A")
	compute := func() ([]byte, error) { calls++; return []byte(`{"text":"A"}`), nil }
	out, err := first.ComputeMember(context.Background(), key, compute)
	if err != nil {
		t.Fatal(err)
	}
	out[0] = '!'
	peer := memberComputationEngine(directory)
	again, err := peer.ComputeMember(context.Background(), key, compute)
	if err != nil || calls != 1 || !bytes.Equal(again, []byte(`{"text":"A"}`)) {
		t.Fatalf("cold immutable reuse: %s calls=%d %v", again, calls, err)
	}
	if _, err := peer.embedBatchWithCache(semanticComputationDocs("vector input"), immediateComputation); err != nil {
		t.Fatal(err)
	}
	if err := memberValueCache(peer).globalTransaction(func(models, meta *bolt.Bucket) error {
		if models.Bucket([]byte("member-values-v1")) == nil || models.Bucket([]byte(peer.embeddingCacheIdentity)) == nil {
			t.Fatal("member and vector partitions did not share the instance authority")
		}
		if cacheBytes(meta) > computationCacheLimit {
			t.Fatal("shared computation budget exceeded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredMemberOwnerCannotReplaceNewComputation(t *testing.T) {
	e := memberComputationEngine(t.TempDir())
	cache := memberValueCache(e)
	key := string(kernel.CanonicalDigest("same-input"))
	_, old, err := cache.claimEntries([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	expireComputationEntries(t, cache)
	_, current, err := cache.claimEntries([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	if old[key] == "" || current[key] == "" || old[key] == current[key] {
		t.Fatal("expired computation did not get a fresh owner")
	}
	result := func(value string) map[string]computationCacheEntry {
		raw := []byte(value)
		return map[string]computationCacheEntry{key: {Value: raw, Digest: kernel.CanonicalDigest(raw)}}
	}
	if err := cache.publishEntries(old, result(`{"value":"old"}`)); err != nil {
		t.Fatal(err)
	}
	if err := cache.publishEntries(current, result(`{"value":"current"}`)); err != nil {
		t.Fatal(err)
	}
	out, err := e.ComputeMember(context.Background(), kernel.Digest(key), func() ([]byte, error) { t.Fatal("accepted current result was not reusable"); return nil, nil })
	if err != nil || string(out) != `{"value":"current"}` {
		t.Fatalf("late owner changed result: %s %v", out, err)
	}
}

func TestMemberComputationCancellationAndReadOnlyViewNeverPublish(t *testing.T) {
	e := memberComputationEngine(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	key := kernel.CanonicalDigest("cancel")
	_, err := e.ComputeMember(ctx, key, func() ([]byte, error) { cancel(); return []byte(`{"value":"unused"}`), nil })
	if err != context.Canceled {
		t.Fatalf("canceled computation: %v", err)
	}
	hits, _, err := memberValueCache(e).claimEntries([]string{string(key)})
	if err != nil || len(hits) != 0 {
		t.Fatalf("canceled result became reusable: %v %v", hits, err)
	}
	readonly := memberComputationEngine(t.TempDir())
	readonly.fixedControl = []byte(`{}`)
	_, err = readonly.ComputeMember(context.Background(), key, func() ([]byte, error) { t.Fatal("read-only query computed member values"); return nil, nil })
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("read-only computation: %v", err)
	}
	files, err := os.ReadDir(readonly.computationCacheDirectory)
	if err != nil || len(files) != 0 {
		t.Fatalf("read-only view touched cache: %v %v", files, err)
	}
}

func TestMemberComputationMediumLossNeverRecreatesLateOwner(t *testing.T) {
	e := memberComputationEngine(t.TempDir())
	cache := memberValueCache(e)
	key := string(kernel.CanonicalDigest("lost-medium"))
	_, owners, err := cache.claimEntries([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.computationCacheDirectory, "vectors.db")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	value := []byte(`{"value":"late"}`)
	err = cache.publishEntries(owners, map[string]computationCacheEntry{key: {Value: value, Digest: kernel.CanonicalDigest(value)}})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("lost medium treated as empty: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("late owner recreated medium: %v", err)
	}
	fresh, err := e.ComputeMember(context.Background(), kernel.Digest(key), func() ([]byte, error) { return []byte(`{"value":"fresh"}`), nil })
	if err != nil || string(fresh) != `{"value":"fresh"}` {
		t.Fatalf("explicit fresh computation: %s %v", fresh, err)
	}
}

func TestMemberPartitionMaintenanceRejectsUnknownShapeAndReclaimsKnownValues(t *testing.T) {
	e := memberComputationEngine(t.TempDir())
	cache := memberValueCache(e)
	key := kernel.CanonicalDigest("member")
	if _, err := e.ComputeMember(context.Background(), key, func() ([]byte, error) { return []byte(`{"value":"A"}`), nil }); err != nil {
		t.Fatal(err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		var entry computationCacheEntry
		if err := json.Unmarshal(records.Get([]byte(key)), &entry); err != nil {
			return err
		}
		entry.Value = []byte(`{"value":"changed"}`)
		_, err := cache.put(records, meta, string(key), entry)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ComputeMember(context.Background(), key, func() ([]byte, error) { t.Fatal("corrupt reusable value silently repaired"); return nil, nil }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt member integrity: %v", err)
	}
}

func TestMemberComputationMaintenanceIsIndependentOfCurrentVectorModel(t *testing.T) {
	root := t.TempDir()
	raw, err := Open(Config{URL: "http://127.0.0.1:1", Namespace: "member-maintenance"})(root, "kr://test/member-maintenance")
	if err != nil {
		t.Fatal(err)
	}
	e := raw.(*openSearchEngine)
	defer e.Close()
	cache := memberValueCache(e)
	key := kernel.CanonicalDigest("expired-value")
	if _, err := e.ComputeMember(context.Background(), key, func() ([]byte, error) { return []byte(`{"value":"ready"}`), nil }); err != nil {
		t.Fatal(err)
	}
	expireComputationEntries(t, cache)
	// A fresh coordinator has no query engines or embedding model to enumerate.
	coordinator, err := OpenResources(Config{URL: e.base, Namespace: e.namespace})
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	maintainer := coordinator.(interface {
		RecoverComputations(context.Context, string) (ports.ResourceRecoveryReport, error)
	})
	report, err := maintainer.RecoverComputations(context.Background(), root)
	if err != nil || len(report.Deleted) != 1 || report.Deleted[0] != "member-values-v1" || len(report.Retained) != 0 {
		t.Fatalf("idle member partition was not reclaimed: %#v %v", report, err)
	}
	if err := cache.globalTransaction(func(models, meta *bolt.Bucket) error {
		if models.Bucket([]byte("member-values-v1")) != nil || cacheBytes(meta) != 0 {
			t.Fatal("deleted member values retained instance budget")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
