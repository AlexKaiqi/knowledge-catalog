package opensearchimpl

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/internal/boltfile"
	"kc/kernel"
)

const computationCacheLimit = 64 << 20
const computationModelReservation = 512

type computationCache struct {
	directory, identity, authority string
	dimensions                     int
	memberValues                   bool
	existingOnly                   bool
	ctx                            context.Context
}
type computationCacheEntry struct {
	Version int           `json:"version"`
	Key     string        `json:"key"`
	Owner   string        `json:"owner,omitempty"`
	Until   int64         `json:"until"`
	Vector  []float32     `json:"vector,omitempty"`
	Value   []byte        `json:"value,omitempty"`
	Digest  kernel.Digest `json:"digest,omitempty"`
}

func validEmbeddingVector(vector []float32, dimensions int) bool {
	if len(vector) != dimensions {
		return false
	}
	for _, v := range vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}
func (c computationCache) decode(key, value []byte) (computationCacheEntry, error) {
	var entry computationCacheEntry
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return entry, kernel.Fail(kernel.ErrPreconditionFailed, "invalid computation record: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return entry, kernel.Fail(kernel.ErrPreconditionFailed, "computation record has trailing content")
	}

	validPayload := len(entry.Value) == 0 && validEmbeddingVector(entry.Vector, c.dimensions) && entry.Digest == kernel.CanonicalDigest(entry.Vector)
	if c.memberValues {
		validPayload = len(entry.Vector) == 0 && json.Valid(entry.Value) && entry.Digest == kernel.CanonicalDigest(entry.Value)
	}
	if entry.Version != c.recordVersion() || entry.Key != string(key) || entry.Until <= 0 ||
		(entry.Owner != "" && (!validGeneration(entry.Owner) || len(entry.Vector) != 0 || len(entry.Value) != 0 || entry.Digest != "")) ||
		(entry.Owner == "" && !validPayload) {
		return entry, kernel.Fail(kernel.ErrPreconditionFailed, "computation record is invalid")
	}

	return entry, nil
}
func (c computationCache) globalTransaction(change func(*bolt.Bucket, *bolt.Bucket) error) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(c.directory, "vectors.db")
	_, statErr := os.Stat(path)
	fresh := os.IsNotExist(statErr)
	if statErr != nil && !fresh {
		return statErr
	}
	if fresh && c.existingOnly {
		if _, err := os.Lstat(c.directory); err == nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "computation cache medium is missing from an existing instance")
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if !c.existingOnly {
		if err := os.MkdirAll(c.directory, 0700); err != nil {
			return err
		}
	}
	timeout := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return ctx.Err()
		}
	}
	options := &bolt.Options{Timeout: timeout}
	if c.existingOnly {
		options.OpenFile = func(path string, flag int, mode os.FileMode) (*os.File, error) {
			return os.OpenFile(path, flag&^os.O_CREATE, mode)
		}
	}
	db, err := boltfile.Open(path, 0600, options)
	if err != nil {
		return err
	}
	defer db.Close()
	authority := c.authority
	if authority == "" {
		authority = c.identity
	}
	return db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		models, meta := tx.Bucket([]byte("models")), tx.Bucket([]byte("authority"))
		if models == nil && meta == nil {
			if !fresh {
				return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache authority was lost")
			}
			foreign := false
			if err := tx.ForEach(func([]byte, *bolt.Bucket) error { foreign = true; return nil }); err != nil {
				return err
			}
			if foreign {
				return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache has unknown ownership")
			}
			models, err = tx.CreateBucket([]byte("models"))
			if err != nil {
				return err
			}
			meta, err = tx.CreateBucket([]byte("authority"))
			if err != nil {
				return err
			}
			if err := meta.Put([]byte("identity"), []byte(authority)); err != nil {
				return err
			}
			if err := meta.Put([]byte("version"), []byte{2}); err != nil {
				return err
			}
			if err := putCacheBytes(meta, 0); err != nil {
				return err
			}
		}
		if models == nil || meta == nil || string(meta.Get([]byte("identity"))) != authority || string(meta.Get([]byte("version"))) != string([]byte{2}) || len(meta.Get([]byte("bytes"))) != 8 || cacheBytes(meta) > computationCacheLimit {
			return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache instance authority is missing or incompatible")
		}
		if err := meta.ForEach(func(key, value []byte) error {
			if value == nil || (string(key) != "identity" && string(key) != "version" && string(key) != "bytes" && string(key) != "after") {
				return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache instance metadata has unknown shape")
			}
			return nil
		}); err != nil {
			return err
		}
		if err := change(models, meta); err != nil {
			return err
		}
		return ctx.Err()
	})
}
func (c computationCache) transaction(change func(*bolt.Bucket, *bolt.Bucket) error) error {
	return c.globalTransaction(func(models, rootMeta *bolt.Bucket) error {
		model := models.Bucket([]byte(c.identity))
		if model == nil {
			if c.existingOnly {
				return nil
			}
			if cacheBytes(rootMeta)+computationModelReservation > computationCacheLimit {
				return nil
			}
			if err := putCacheBytes(rootMeta, cacheBytes(rootMeta)+computationModelReservation); err != nil {
				return err
			}
			var err error
			model, err = models.CreateBucket([]byte(c.identity))
			if err != nil {
				return err
			}
			if _, err := model.CreateBucket([]byte("vectors")); err != nil {
				return err
			}
			meta, err := model.CreateBucket([]byte("authority"))
			if err != nil {
				return err
			}
			if err := putCacheBytes(meta, 0); err != nil {
				return err
			}
			var dims [8]byte
			binary.BigEndian.PutUint64(dims[:], uint64(c.dimensions))
			if err := meta.Put([]byte("dimensions"), dims[:]); err != nil {
				return err
			}
			if c.memberValues {
				if err := meta.Put([]byte("kind"), []byte("member-values-v1")); err != nil {
					return err
				}
			}
		}
		records, meta, _, err := c.partition(model, rootMeta)
		if err != nil {
			return err
		}
		return change(records, meta)
	})
}
func changeCacheBytes(meta *bolt.Bucket, oldSize, newSize uint64) error {
	global := meta.Tx().Bucket([]byte("authority"))
	localTotal, globalTotal := cacheBytes(meta), cacheBytes(global)
	if localTotal < oldSize || globalTotal < oldSize {
		return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache accounting is invalid")
	}
	if err := putCacheBytes(meta, localTotal-oldSize+newSize); err != nil {
		return err
	}
	return putCacheBytes(global, globalTotal-oldSize+newSize)
}
func cacheBytes(meta *bolt.Bucket) uint64 { return binary.BigEndian.Uint64(meta.Get([]byte("bytes"))) }
func putCacheBytes(meta *bolt.Bucket, total uint64) error {
	var out [8]byte
	binary.BigEndian.PutUint64(out[:], total)
	return meta.Put([]byte("bytes"), out[:])
}
func (c computationCache) put(records, meta *bolt.Bucket, key string, entry computationCacheEntry) (bool, error) {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return false, err
	}
	old := records.Get([]byte(key))
	total := cacheBytes(meta)
	oldSize := uint64(0)
	if old != nil {
		oldSize = uint64(len(key) + len(old))
	}
	if total < oldSize || cacheBytes(meta.Tx().Bucket([]byte("authority"))) < oldSize {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache accounting is invalid")
	}
	next := cacheBytes(meta.Tx().Bucket([]byte("authority"))) - oldSize + uint64(len(key)+len(encoded))
	if next > computationCacheLimit {
		return false, nil
	}
	if err := records.Put([]byte(key), encoded); err != nil {
		return false, err
	}
	return true, changeCacheBytes(meta, oldSize, uint64(len(key)+len(encoded)))
}

// Scan a bounded durable partition. READY values are independent copies, so
// capacity eviction cannot invalidate published queries or in-flight readers.
func (c computationCache) prune(records, meta *bolt.Bucket, needed uint64) error {
	cursor := records.Cursor()
	after := append([]byte(nil), meta.Get([]byte("after"))...)
	key, value := cursor.First()
	if len(after) > 0 {
		key, value = cursor.Seek(after)
		if key != nil && string(key) == string(after) {
			key, value = cursor.Next()
		}
	}
	now := time.Now().UnixMilli()
	type removal struct {
		key  []byte
		size uint64
	}
	removals := []removal{}
	last := []byte(nil)
	total := cacheBytes(meta.Tx().Bucket([]byte("authority")))
	local := cacheBytes(meta)
	for n := 0; n < 256 && key != nil; n++ {
		entry, err := c.decode(key, value)
		if err != nil {
			return err
		}
		last = append(last[:0], key...)
		if entry.Until <= now || (entry.Owner == "" && total+needed > computationCacheLimit) {
			size := uint64(len(key) + len(value))
			if total < size || local < size {
				return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache accounting is invalid")
			}
			total -= size
			local -= size
			removals = append(removals, removal{append([]byte(nil), key...), size})
		}
		key, value = cursor.Next()
	}
	if key == nil {
		last = nil
	}
	// Validate the entire bounded batch before mutation. A corrupt record cannot
	// cause partial deletion or starve independent model partitions.
	for _, removal := range removals {
		if err := records.Delete(removal.key); err != nil {
			return err
		}
		if err := changeCacheBytes(meta, removal.size, 0); err != nil {
			return err
		}
	}
	return meta.Put([]byte("after"), last)
}

func (c computationCache) partition(model, rootMeta *bolt.Bucket) (*bolt.Bucket, *bolt.Bucket, int, error) {
	if model == nil {
		return nil, nil, 0, kernel.Fail(kernel.ErrPreconditionFailed, "embedding model partition is missing")
	}
	if err := model.ForEach(func(key, value []byte) error {
		if value != nil || (string(key) != "vectors" && string(key) != "authority") {
			return kernel.Fail(kernel.ErrPreconditionFailed, "embedding model contains unknown data")
		}
		return nil
	}); err != nil {
		return nil, nil, 0, err
	}
	records, meta := model.Bucket([]byte("vectors")), model.Bucket([]byte("authority"))
	if records == nil || meta == nil || len(meta.Get([]byte("dimensions"))) != 8 || len(meta.Get([]byte("bytes"))) != 8 || cacheBytes(meta) > cacheBytes(rootMeta) {
		return nil, nil, 0, kernel.Fail(kernel.ErrPreconditionFailed, "embedding model partition authority is missing or incompatible")
	}
	dimensions := binary.BigEndian.Uint64(meta.Get([]byte("dimensions")))
	kind := string(meta.Get([]byte("kind")))
	memberValues := kind == "member-values-v1" && dimensions == 0
	if (kind != "" && !memberValues) || (!memberValues && (dimensions < 1 || dimensions > 16384)) || (c.memberValues && !memberValues) || (c.dimensions > 0 && uint64(c.dimensions) != dimensions) {
		return nil, nil, 0, kernel.Fail(kernel.ErrPreconditionFailed, "embedding model dimensions are incompatible")
	}
	if err := meta.ForEach(func(key, value []byte) error {
		if value == nil || (string(key) != "bytes" && string(key) != "dimensions" && string(key) != "after" && string(key) != "kind") {
			return kernel.Fail(kernel.ErrPreconditionFailed, "embedding model metadata has unknown shape")
		}
		return nil
	}); err != nil {
		return nil, nil, 0, err
	}
	return records, meta, int(dimensions), nil
}

func (c computationCache) claimEntries(keys []string) (map[string]computationCacheEntry, map[string]string, error) {
	hits, owners := map[string]computationCacheEntry{}, map[string]string{}
	err := c.transaction(func(records, meta *bolt.Bucket) error {
		if err := c.prune(records, meta, 0); err != nil {
			return err
		}
		now := time.Now()
		for _, key := range keys {
			if raw := records.Get([]byte(key)); raw != nil {
				entry, err := c.decode([]byte(key), raw)
				if err != nil {
					return err
				}
				if entry.Until > now.UnixMilli() {
					if entry.Owner == "" {
						hits[key] = entry
						entry.Until = now.Add(time.Hour).UnixMilli()
						if _, err := c.put(records, meta, key, entry); err != nil {
							return err
						}
					}
					continue
				}
			}
			owner, err := newResourceID()
			if err != nil {
				return err
			}
			stored, err := c.put(records, meta, key, computationCacheEntry{Version: c.recordVersion(), Key: key, Owner: owner, Until: now.Add(buildLease).UnixMilli()})
			if err != nil {
				return err
			}
			if stored {
				owners[key] = owner
			}
		}
		return nil
	})
	return hits, owners, err
}
func (c computationCache) publishEntries(owners map[string]string, results map[string]computationCacheEntry) error {
	c.existingOnly = true
	return c.transaction(func(records, meta *bolt.Bucket) error {
		// A full batch is bounded by the provider's existing 500-document limit.
		needed := uint64(0)
		for key := range owners {
			encoded, err := json.Marshal(results[key])
			if err != nil {
				return err
			}
			needed += uint64(len(encoded) + len(key) + 128)
		}
		if err := c.prune(records, meta, needed); err != nil {
			return err
		}
		now := time.Now()
		for key, owner := range owners {
			raw := records.Get([]byte(key))
			if raw == nil {
				continue
			}
			entry, err := c.decode([]byte(key), raw)
			if err != nil {
				return err
			}
			if entry.Owner != owner || entry.Until <= now.UnixMilli() {
				continue
			}
			ready := results[key]
			ready.Version, ready.Key, ready.Owner, ready.Until = c.recordVersion(), key, "", now.Add(time.Hour).UnixMilli()
			encoded, err := json.Marshal(ready)
			if err != nil {
				return err
			}
			if _, err := c.decode([]byte(key), encoded); err != nil {
				return err
			}
			_, err = c.put(records, meta, key, ready)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Only the exact unpublished owner can cancel its staging record. An unknown
// publication result or a concurrent winner is never overwritten by cleanup.
func (c computationCache) cancel(owners map[string]string) error {
	c.existingOnly = true
	return c.transaction(func(records, meta *bolt.Bucket) error {
		for key, owner := range owners {
			raw := records.Get([]byte(key))
			if raw == nil {
				continue
			}
			entry, err := c.decode([]byte(key), raw)
			if err != nil {
				return err
			}
			if entry.Owner != owner {
				continue
			}
			size, total := uint64(len(key)+len(raw)), cacheBytes(meta)
			if total < size {
				return kernel.Fail(kernel.ErrPreconditionFailed, "embedding cache accounting is invalid")
			}
			if err := records.Delete([]byte(key)); err != nil {
				return err
			}
			if err := changeCacheBytes(meta, size, 0); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c computationCache) recordVersion() int {
	if c.memberValues {
		return 2
	}
	return 1
}
func (c computationCache) claim(keys []string) (map[string][]float32, map[string]string, error) {
	entries, owners, err := c.claimEntries(keys)
	hits := map[string][]float32{}
	for key, entry := range entries {
		hits[key] = append([]float32(nil), entry.Vector...)
	}
	return hits, owners, err
}
func (c computationCache) publish(owners map[string]string, vectors map[string][]float32) error {
	results := map[string]computationCacheEntry{}
	for key, vector := range vectors {
		results[key] = computationCacheEntry{Vector: vector, Digest: kernel.CanonicalDigest(vector)}
	}
	return c.publishEntries(owners, results)
}
