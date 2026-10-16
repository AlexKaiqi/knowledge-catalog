package writer

import (
	"sync"
	"sync/atomic"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// basisReadConcurrency bounds the point reads one commit issues against its
// immutable planning basis. It matches the Snapshot staging fan-out so reads
// and writes keep the same per-host pressure.
const basisReadConcurrency = 32

// basisReadCache is an internal planning view, never the write authority.
// Commits are immutable, so a file read at a fixed commit (including "missing")
// answers every later read of the same coordinate inside one Writer command.
// Schema planning, the tree codec and path admission share one view instead of
// each paying its own round trips. Transport failures are never cached.
type basisReadCache struct {
	snapshot.Store
	snapshot.TreeStore
	mu    sync.Mutex
	files map[basisCoordinate]basisFile
}

type basisCoordinate struct {
	commit kernel.CommitID
	path   string
}

type basisFile struct {
	content []byte
	err     error
}

// cachedReadBasis wraps a tree-backed basis for the duration of one command.
// Native object authorities keep their own reads.
func cachedReadBasis(basis snapshot.Store) snapshot.Store {
	if _, native := basis.(objectmodel.NativeRepository); native {
		return basis
	}
	tree, ok := snapshot.TreeStoreOf(basis)
	if !ok {
		return basis
	}
	return &basisReadCache{Store: basis, TreeStore: tree, files: map[basisCoordinate]basisFile{}}
}

func (c *basisReadCache) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if at == "" {
		return c.TreeStore.ReadFile(path, at)
	}
	key := basisCoordinate{commit: at, path: path}
	c.mu.Lock()
	hit, ok := c.files[key]
	c.mu.Unlock()
	if ok {
		return cloneBytes(hit.content), hit.err
	}
	raw, err := c.TreeStore.ReadFile(path, at)
	if err == nil || kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		c.mu.Lock()
		c.files[key] = basisFile{content: cloneBytes(raw), err: err}
		c.mu.Unlock()
	}
	return raw, err
}

func (c *basisReadCache) cached(key basisCoordinate) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.files[key]
	return ok
}

// prefetch warms independent coordinates with bounded concurrency. It only
// fills the cache: callers still read in their own order and receive any
// failure from that read, so error semantics stay those of the serial loop.
// The first transport failure stops new prefetch work.
func (c *basisReadCache) prefetch(paths []string, at kernel.CommitID) {
	if at == "" {
		return
	}
	seen := map[string]struct{}{}
	pending := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		if !c.cached(basisCoordinate{commit: at, path: path}) {
			pending = append(pending, path)
		}
	}
	workers := basisReadConcurrency
	if workers > len(pending) {
		workers = len(pending)
	}
	if workers <= 1 {
		return
	}
	var (
		next   int64
		failed atomic.Bool
		wg     sync.WaitGroup
	)
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for !failed.Load() {
				index := int(atomic.AddInt64(&next, 1)) - 1
				if index >= len(pending) {
					return
				}
				if _, err := c.ReadFile(pending[index], at); err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
					failed.Store(true)
				}
			}
		}()
	}
	wg.Wait()
}

// basisPrefetcher is satisfied only by the planning view above; plain trees
// keep the serial on-demand reads.
type basisPrefetcher interface {
	prefetch(paths []string, at kernel.CommitID)
}

func prefetchBasis(tree snapshot.TreeReader, paths []string, at kernel.CommitID) {
	if warm, ok := tree.(basisPrefetcher); ok {
		warm.prefetch(paths, at)
	}
}

func cloneBytes(raw []byte) []byte {
	if raw == nil {
		return nil
	}
	return append([]byte(nil), raw...)
}
