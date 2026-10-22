package opensearchimpl

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

type countedEmbedding struct {
	mu     sync.Mutex
	inputs []string
	model  string
}

func (e *countedEmbedding) Embed(_ context.Context, r retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inputs = append(e.inputs, r.Texts...)
	vectors := make([][]float32, len(r.Texts))
	for i, text := range r.Texts {
		vectors[i] = []float32{float32(len(text)), 1}
	}
	return retrieval.EmbeddingResult{Vectors: vectors, Model: e.model, Provider: "counted", Dimensions: 2}, nil
}
func (e *countedEmbedding) count() int { e.mu.Lock(); defer e.mu.Unlock(); return len(e.inputs) }
func computationEngine(directory string, model *countedEmbedding) *openSearchEngine {
	return &openSearchEngine{namespace: "owned", embedder: model, vectorDimension: 2, vectorModel: "test@1", computationCacheDirectory: directory, embeddingIdentity: "configured-model-v1", embeddingCacheIdentity: "configured-model-v1"}
}
func immediateComputation(operation func() error) error { return operation() }
func semanticComputationDocs(text string) []ports.CompiledDoc {
	return []ports.CompiledDoc{{Semantic: []retrieval.SemanticField{{Member: "one", Model: "test@1", Text: text}, {Member: "two", Model: "test@1", Text: text}}}}
}

func TestEmbeddingComputationCachePersistsVectorsAndKeepsMemberSlotsIndependent(t *testing.T) {
	directory := t.TempDir()
	model := &countedEmbedding{model: "test@1"}
	first := computationEngine(directory, model)
	out, err := first.embedBatchWithCache(semanticComputationDocs("input A"), immediateComputation)
	if err != nil || model.count() != 1 || !reflect.DeepEqual(out, [][][]float32{{{7, 1}, {7, 1}}}) {
		t.Fatalf("initial member computation: %v calls=%d %v", out, model.count(), err)
	}
	out[0][0][0] = 999
	second := computationEngine(directory, model)
	again, err := second.embedBatchWithCache(semanticComputationDocs("input A"), immediateComputation)
	if err != nil || model.count() != 1 || again[0][0][0] != 7 || again[0][1][0] != 7 {
		t.Fatalf("cold reuse mutated source artifact: %v calls=%d %v", again, model.count(), err)
	}
	if _, err := second.embedBatchWithCache(semanticComputationDocs("B"), immediateComputation); err != nil {
		t.Fatal(err)
	}
	if _, err := second.embedBatchWithCache(semanticComputationDocs("input A"), immediateComputation); err != nil || model.count() != 2 {
		t.Fatalf("A-B-A was not reusable: calls=%d %v", model.count(), err)
	}
}
func TestEmbeddingComputationCacheCannotPublishAfterBuildAdmissionCloses(t *testing.T) {
	engine := computationEngine(t.TempDir(), &countedEmbedding{model: "test@1"})
	calls := 0
	denied := errors.New("build authority closed")
	guard := func(operation func() error) error {
		calls++
		if calls >= 2 {
			return denied
		}
		return operation()
	}
	if _, err := engine.embedBatchWithCache(semanticComputationDocs("late"), guard); !errors.Is(err, denied) {
		t.Fatalf("late result was published: %v", err)
	}
	model := engine.embedder.(*countedEmbedding)
	if _, err := engine.embedBatchWithCache(semanticComputationDocs("late"), immediateComputation); err != nil || model.count() != 2 {
		t.Fatalf("unpublished result was reused: %d %v", model.count(), err)
	}
}
func TestEmbeddingComputationLeaseFencePreventsDeletedOwnerResurrection(t *testing.T) {
	cache := computationCache{directory: t.TempDir(), identity: "model", dimensions: 2}
	_, old, err := cache.claim([]string{"key"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		entry, err := cache.decode([]byte("key"), records.Get([]byte("key")))
		if err != nil {
			return err
		}
		entry.Until = time.Now().Add(-time.Minute).UnixMilli()
		_, err = cache.put(records, meta, "key", entry)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := cache.claim([]string{"key"})
	if err != nil || old["key"] == fresh["key"] {
		t.Fatalf("expired owner was renewed: %v", err)
	}
	if err := cache.publish(old, map[string][]float32{"key": {99, 99}}); err != nil {
		t.Fatal(err)
	}
	hits, _, err := cache.claim([]string{"key"})
	if err != nil || hits["key"] != nil {
		t.Fatalf("old owner resurrected value: %v %v", hits, err)
	}
	if err := cache.publish(fresh, map[string][]float32{"key": {1, 2}}); err != nil {
		t.Fatal(err)
	}
	hits, _, err = cache.claim([]string{"key"})
	if err != nil || !reflect.DeepEqual(hits["key"], []float32{1, 2}) {
		t.Fatalf("fresh owner failed: %v %v", hits, err)
	}
}
func TestEmbeddingComputationRejectsUnknownAuthorityAndNonFiniteVectors(t *testing.T) {
	cache := computationCache{directory: t.TempDir(), identity: "model-one", dimensions: 2}
	if _, _, err := cache.claim([]string{"key"}); err != nil {
		t.Fatal(err)
	}
	other := cache
	other.identity = "model-two"
	if _, _, err := other.claim([]string{"key"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("different model adopted file: %v", err)
	}
	if validEmbeddingVector([]float32{float32(math.NaN()), 1}, 2) || validEmbeddingVector([]float32{1}, 2) {
		t.Fatal("invalid vector accepted")
	}
}

func expireComputationEntries(t *testing.T, cache computationCache) {
	t.Helper()
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		keys := [][]byte{}
		if err := records.ForEach(func(key, value []byte) error { keys = append(keys, append([]byte(nil), key...)); return nil }); err != nil {
			return err
		}
		for _, key := range keys {
			entry, err := cache.decode(key, records.Get(key))
			if err != nil {
				return err
			}
			entry.Until = time.Now().Add(-time.Minute).UnixMilli()
			if _, err := cache.put(records, meta, string(key), entry); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestEmbeddingComputationCleanupIsBoundedAndResumesDurably(t *testing.T) {
	cache := computationCache{directory: t.TempDir(), identity: "bounded-model", dimensions: 2}
	keys := make([]string, 1000)
	vectors := map[string][]float32{}
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
		vectors[keys[i]] = []float32{1, 2}
	}
	_, owners, err := cache.claim(keys)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.publish(owners, vectors); err != nil {
		t.Fatal(err)
	}
	expireComputationEntries(t, cache)
	if _, _, err := cache.claim(nil); err != nil {
		t.Fatal(err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		if records.Stats().KeyN != 744 {
			t.Fatalf("cleanup exceeded 256-record pass: %d", records.Stats().KeyN)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err := cache.claim(nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		if records.Stats().KeyN != 0 || cacheBytes(meta) != 0 {
			t.Fatalf("cleanup did not converge: %d records %d bytes", records.Stats().KeyN, cacheBytes(meta))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestEmbeddingComputationCapacityRefusesNewRecordWithoutOverwritingAuthority(t *testing.T) {
	cache := computationCache{directory: t.TempDir(), identity: "capacity-model", dimensions: 2}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		if err := putCacheBytes(meta.Tx().Bucket([]byte("authority")), computationCacheLimit); err != nil {
			return err
		}
		if err := putCacheBytes(meta, computationCacheLimit); err != nil {
			return err
		}
		stored, err := cache.put(records, meta, "too-large", computationCacheEntry{Version: 1, Key: "too-large", Owner: "0123456789abcdef0123456789abcdef", Until: time.Now().Add(time.Minute).UnixMilli()})
		if err != nil {
			return err
		}
		if stored || records.Get([]byte("too-large")) != nil || cacheBytes(meta) != computationCacheLimit {
			t.Fatal("cache exceeded logical capacity")
		}
		if err := putCacheBytes(meta.Tx().Bucket([]byte("authority")), computationModelReservation); err != nil {
			return err
		}
		return putCacheBytes(meta, 0)
	}); err != nil {
		t.Fatal(err)
	}
}
func TestEmbeddingConfigurationChangesInvalidateComputationAndPhysicalCompatibility(t *testing.T) {
	t.Setenv(EnvEmbeddingModel, "test@1")
	t.Setenv(EnvEmbeddingDims, "2")
	t.Setenv("OPENAI_BASE_URL", "https://embedding.example/v1")
	t.Setenv("OPENAI_API_KEY", "test-secret-one")
	directory := t.TempDir()
	cfg := Config{URL: "http://search.example:9200", Namespace: "owned"}
	open := func() *openSearchEngine {
		engine, err := Open(cfg)(directory, "scope")
		if err != nil {
			t.Fatal(err)
		}
		return engine.(*openSearchEngine)
	}
	first := open()
	same := open()
	if first.embeddingIdentity == "" || first.embeddingIdentity != same.embeddingIdentity || first.computationCacheDirectory != same.computationCacheDirectory || first.PhysicalDigest() != same.PhysicalDigest() {
		t.Fatal("identical model configuration did not retain identity")
	}
	if _, err := os.Stat(first.computationCacheDirectory); !os.IsNotExist(err) {
		t.Fatalf("opening query handle created computation resources: %v", err)
	}
	for _, setting := range []struct{ name, value string }{{"OPENAI_BASE_URL", "https://different.example/v1"}, {"OPENAI_API_KEY", "test-secret-two"}, {EnvEmbeddingModel, "test@2"}, {EnvEmbeddingDims, "3"}} {
		t.Setenv(setting.name, setting.value)
		next := open()
		if next.embeddingCacheIdentity == first.embeddingCacheIdentity || next.computationCacheDirectory != first.computationCacheDirectory {
			t.Fatalf("configuration change reused old model: %s", setting.name)
		}
		semanticChanged := setting.name != "OPENAI_API_KEY"
		if (next.embeddingIdentity != first.embeddingIdentity) != semanticChanged || (next.PhysicalDigest() != first.PhysicalDigest()) != semanticChanged {
			t.Fatalf("credential rotation changed semantic compatibility or actual model change was ignored: %s", setting.name)
		}
		if strings.Contains(next.embeddingIdentity, "test-secret") || strings.Contains(next.embeddingCacheIdentity, "test-secret") || strings.Contains(next.computationCacheDirectory, "test-secret") {
			t.Fatal("credential text persisted in cache identity")
		}
		first = next
	}
}

func TestEmbeddingComputationFailureReleasesOnlyUnpublishedTicket(t *testing.T) {
	model := &countedEmbedding{model: "different-model"}
	engine := computationEngine(t.TempDir(), model)
	if _, err := engine.embedBatchWithCache(semanticComputationDocs("same-input"), immediateComputation); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("different effective model was accepted: %v", err)
	}
	model.model = "test@1"
	if _, err := engine.embedBatchWithCache(semanticComputationDocs("same-input"), immediateComputation); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.embedBatchWithCache(semanticComputationDocs("same-input"), immediateComputation); err != nil || model.count() != 2 {
		t.Fatalf("failed computation retained an unusable ticket: inputs=%d %v", model.count(), err)
	}
}
func TestEmbeddingComputationMissingAuthorityCannotBeReinitialized(t *testing.T) {
	cache := computationCache{directory: t.TempDir(), identity: "original", dimensions: 2}
	if _, _, err := cache.claim([]string{"key"}); err != nil {
		t.Fatal(err)
	}
	if err := cache.transaction(func(records, meta *bolt.Bucket) error {
		tx := records.Tx()
		return tx.DeleteBucket([]byte("authority"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.claim([]string{"key"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing authority was repaired: %v", err)
	}
}
