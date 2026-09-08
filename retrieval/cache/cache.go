// Package cache implements an optional, bounded consumer-side Snapshot body
// cache. It is neither an authority nor an index and never stores observations.
package cache

import (
	"container/list"
	"encoding/json"
	"reflect"
	"sync"

	"kc/kernel"
	"kc/knowledge"
)

type Config struct {
	MaxBytes      int64 // Accounted retained bytes, including keys and metadata allowances. Default: 64 MiB.
	MaxEntries    int   // Maximum retained bodies. Default: 4096.
	WarmLimit     int   // Maximum distinct object/address reads per maintenance invocation. Default: 128.
	WarmBatchSize int   // Maximum objects per warm authority request. Default: 32.
	ColdStart     bool  // Allow one identity page when no retained hot objects exist.
}

// Stats is a consistent process-local snapshot. Bytes counts encoded payload
// size plus key and metadata allowances, not Go heap/RSS. Misses counts unique
// uncached identities. SourceReads counts actual ReadMany, Read or ReadAddress
// authority calls, including failed calls. Errors and absence are never cached.
type Stats struct {
	Hits, Misses, SourceReads, Evictions, Bypasses uint64
	WarmRuns, WarmObjects, WarmFailures            uint64
	Entries, WarmMarkers                           int
	Bytes                                          int64
	Closed                                         bool
}

type key struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	address    knowledge.Address
	object     bool // Whole-object aggregation differs from an Entity unit read.
}

type entry struct {
	key   key
	value knowledge.KnowledgeValue
	bytes int64
}

type warmKey struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
}

type warmMarker struct {
	key   warmKey
	bytes int64
}

type Cache struct {
	mu      sync.Mutex
	warmMu  sync.Mutex
	config  Config
	entries map[key]*list.Element
	lru     *list.List
	warmed  map[warmKey]*list.Element
	warmLRU *list.List
	stats   Stats
	epoch   uint64 // Clear/Close prevent in-flight requests from refilling old state.
}

var _ knowledge.Hydrator = (*Cache)(nil)

func New(config Config) (*Cache, error) {
	if config.MaxBytes < 0 || config.MaxEntries < 0 || config.WarmLimit < 0 || config.WarmBatchSize < 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "cache limits cannot be negative")
	}
	if config.MaxBytes == 0 {
		config.MaxBytes = 64 << 20
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = 4096
	}
	if config.WarmLimit == 0 {
		config.WarmLimit = 128
	}
	if config.WarmBatchSize == 0 {
		config.WarmBatchSize = 32
	}
	if config.WarmLimit > config.MaxEntries {
		config.WarmLimit = config.MaxEntries
	}
	if config.WarmBatchSize > 1000 {
		config.WarmBatchSize = 1000
	}
	return &Cache{config: config, entries: map[key]*list.Element{}, lru: list.New(), warmed: map[warmKey]*list.Element{}, warmLRU: list.New()}, nil
}

func validateRequest(repo knowledge.Repository, commit kernel.CommitID) error {
	if repo == nil || repo.ID() == "" || commit == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "cache hydration requires a repository and explicit fixed commit")
	}
	return nil
}

func validateValue(v knowledge.KnowledgeValue, k key) error {
	if k.object {
		return knowledge.ValidateHydratedObject(k.repository, k.commit, k.address.ObjectID, v)
	}
	return knowledge.ValidateHydratedAddress(k.repository, k.commit, k.address, v)
}

func objectKey(repo kernel.RepositoryID, commit kernel.CommitID, id knowledge.ObjectID) key {
	return key{repository: repo, commit: commit, address: knowledge.Address{ObjectID: id}, object: true}
}

// ReadMany performs one batch authority read for unique misses when the
// repository supports BatchReadStore. It never follows HEAD or caches absence.
func (c *Cache) ReadMany(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	c.mu.Lock()
	epoch := c.epoch
	c.mu.Unlock()
	return c.readMany(repo, commit, ids, epoch)
}

// The caller owns the refill epoch for the entire operation. In particular,
// Warm must preserve its original epoch across every batch and address read.
func (c *Cache) readMany(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID, epoch uint64) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	if err := validateRequest(repo, commit); err != nil {
		return nil, err
	}
	repository := repo.ID()
	out := make(map[knowledge.ObjectID]knowledge.KnowledgeValue, len(ids))
	misses := make([]knowledge.ObjectID, 0, len(ids))
	seen := make(map[knowledge.ObjectID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if v, ok := c.get(objectKey(repository, commit, id)); ok {
			out[id] = clone(v)
		} else {
			misses = append(misses, id)
		}
	}
	if len(misses) == 0 {
		return out, nil
	}
	loaded, err := c.readSource(repo, commit, misses)
	if err != nil {
		return nil, err
	}
	// Validate the entire source result before filling any entry.
	requested := make(map[knowledge.ObjectID]struct{}, len(misses))
	for _, id := range misses {
		requested[id] = struct{}{}
	}
	for id, v := range loaded {
		if _, ok := requested[id]; !ok {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "hydrate authority returned an unrequested object")
		}
		if err := validateValue(v, objectKey(repository, commit, id)); err != nil {
			return nil, err
		}
	}
	for _, id := range misses {
		if v, ok := loaded[id]; ok {
			c.put(objectKey(repository, commit, id), v, epoch)
			out[id] = clone(v)
		}
	}
	return out, nil
}

func (c *Cache) readSource(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	if batch, ok := repo.(knowledge.BatchReadStore); ok {
		c.mu.Lock()
		c.stats.SourceReads++
		c.mu.Unlock()
		return batch.ReadMany(ids, commit)
	}
	out := make(map[knowledge.ObjectID]knowledge.KnowledgeValue, len(ids))
	for _, id := range ids {
		c.mu.Lock()
		c.stats.SourceReads++
		c.mu.Unlock()
		v, err := repo.Read(id, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, nil
}

func (c *Cache) ReadAddress(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (knowledge.KnowledgeValue, error) {
	c.mu.Lock()
	epoch := c.epoch
	c.mu.Unlock()
	return c.readAddress(repo, commit, address, epoch)
}

func (c *Cache) readAddress(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address, epoch uint64) (knowledge.KnowledgeValue, error) {
	if err := validateRequest(repo, commit); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if address.ObjectID == "" {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrUsageInvalid, "cache address requires object identity")
	}
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	k := key{repository: repo.ID(), commit: commit, address: address}
	if v, ok := c.get(k); ok {
		return clone(v), nil
	}
	c.mu.Lock()
	c.stats.SourceReads++
	c.mu.Unlock()
	v, err := repo.ReadAddress(address, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if err := validateValue(v, k); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	c.put(k, v, epoch)
	return clone(v), nil
}

func (c *Cache) get(k key) (knowledge.KnowledgeValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[k]; ok && !c.stats.Closed {
		c.lru.MoveToFront(e)
		c.stats.Hits++
		return e.Value.(entry).value, true
	}
	c.stats.Misses++
	return knowledge.KnowledgeValue{}, false
}

func (c *Cache) put(k key, v knowledge.KnowledgeValue, epoch uint64) {
	c.mu.Lock()
	disabled := c.stats.Closed || c.epoch != epoch
	c.mu.Unlock()
	if disabled {
		return
	}
	encoded, err := json.Marshal(v)
	weight := int64(len(encoded) + len(k.repository) + len(k.commit) + len(k.address.ObjectID) + len(k.address.Kind) + len(k.address.AspectName) + len(k.address.MemberKey) + 256)
	if err != nil || weight > c.config.MaxBytes {
		c.mu.Lock()
		c.stats.Bypasses++
		c.mu.Unlock()
		return
	}
	copy := clone(v)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stats.Closed || c.epoch != epoch {
		return
	}
	if existing, ok := c.entries[k]; ok {
		c.lru.MoveToFront(existing)
		return
	}
	for c.stats.Bytes+weight > c.config.MaxBytes && c.warmLRU.Len() > 0 {
		c.evictMarker()
	}
	for (len(c.entries) >= c.config.MaxEntries || c.stats.Bytes+weight > c.config.MaxBytes) && c.lru.Len() > 0 {
		c.evict()
	}
	c.entries[k] = c.lru.PushFront(entry{key: k, value: copy, bytes: weight})
	c.stats.Bytes += weight
	c.stats.Entries = len(c.entries)
}

func (c *Cache) evict() {
	e := c.lru.Back()
	v := e.Value.(entry)
	delete(c.entries, v.key)
	c.lru.Remove(e)
	c.stats.Bytes -= v.bytes
	c.stats.Evictions++
	c.stats.Entries = len(c.entries)
}

func (c *Cache) Stats() Stats { c.mu.Lock(); defer c.mu.Unlock(); return c.stats }

// Clear drops bodies, warm completion markers and in-flight refill eligibility.
// Counters remain cumulative. Historical requests can refill from authority.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clear()
}
func (c *Cache) clear() {
	c.epoch++
	c.entries = map[key]*list.Element{}
	c.lru.Init()
	c.warmed = map[warmKey]*list.Element{}
	c.warmLRU.Init()
	c.stats.Entries = 0
	c.stats.Bytes = 0
	c.stats.WarmMarkers = 0
}

// Close releases retained state and leaves reads as exact-basis pass-through.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clear()
	c.stats.Closed = true
	return nil
}

// Clone preserves native scalar types (including integers and []byte). A JSON
// roundtrip would change those types. Snapshot payloads are structured values;
// visited pointers also make shared or cyclic source containers safe to copy.
func clone(v knowledge.KnowledgeValue) knowledge.KnowledgeValue {
	return cloneReflect(reflect.ValueOf(v), map[visit]reflect.Value{}).Interface().(knowledge.KnowledgeValue)
}

type visit struct {
	typ     reflect.Type
	pointer uintptr
	length  int
}

func cloneReflect(v reflect.Value, seen map[visit]reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(cloneReflect(v.Elem(), seen))
		return out
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		identity := visit{typ: v.Type(), pointer: v.Pointer()}
		if v.Kind() == reflect.Slice {
			identity.length = v.Len()
		}
		if out, ok := seen[identity]; ok {
			return out
		}
		var out reflect.Value
		switch v.Kind() {
		case reflect.Pointer:
			out = reflect.New(v.Type().Elem())
			seen[identity] = out
			out.Elem().Set(cloneReflect(v.Elem(), seen))
		case reflect.Map:
			out = reflect.MakeMapWithSize(v.Type(), v.Len())
			seen[identity] = out
			iter := v.MapRange()
			for iter.Next() {
				out.SetMapIndex(cloneReflect(iter.Key(), seen), cloneReflect(iter.Value(), seen))
			}
		case reflect.Slice:
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			seen[identity] = out
			for i := 0; i < v.Len(); i++ {
				out.Index(i).Set(cloneReflect(v.Index(i), seen))
			}
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if out.Field(i).CanSet() && v.Field(i).CanInterface() {
				out.Field(i).Set(cloneReflect(v.Field(i), seen))
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneReflect(v.Index(i), seen))
		}
		return out
	default:
		return v
	}
}
