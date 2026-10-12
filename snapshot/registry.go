package snapshot

import (
	"kc/kernel"
	"sync"
)

type registry struct {
	mu         sync.RWMutex
	stores     map[kernel.RepositoryID]Store
	onAdvanced []func(Advanced)
	wrap       func(Store) Store
}

func NewRegistry() Registry {
	return &registry{stores: map[kernel.RepositoryID]Store{}}
}

// SetWrap installs a decorator applied to every registered Store, including
// members already in the registry. The wrap function must be idempotent.
func (r *registry) SetWrap(fn func(Store) Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wrap = fn
	if fn == nil {
		return
	}
	for id, store := range r.stores {
		r.stores[id] = fn(store)
	}
}

func (r *registry) Add(store Store) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.stores[store.ID()]; ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "repository %s is already registered", store.ID())
	}
	if r.wrap != nil {
		store = r.wrap(store)
	}
	r.stores[store.ID()] = store
	return nil
}

func (r *registry) Get(id kernel.RepositoryID) (Store, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	store, ok := r.stores[id]
	return store, ok
}

func (r *registry) Delete(id kernel.RepositoryID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.stores, id)
}

func (r *registry) Require(id kernel.RepositoryID, code kernel.ErrorCode) (Store, error) {
	store, ok := r.Get(id)
	if ok {
		return store, nil
	}
	if code == kernel.ErrUsageInvalid {
		return nil, kernel.Fail(code, "repository %s is not mounted", id)
	}
	return nil, kernel.Fail(code, "unknown repository %s", id)
}

func (r *registry) IDs() []kernel.RepositoryID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]kernel.RepositoryID, 0, len(r.stores))
	for id := range r.stores {
		ids = append(ids, id)
	}
	return ids
}

func (r *registry) Close() error {
	r.mu.Lock()
	stores := r.stores
	r.stores = map[kernel.RepositoryID]Store{}
	r.mu.Unlock()
	var first error
	for _, store := range stores {
		if closer, ok := store.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (r *registry) OnAdvanced(fn func(Advanced)) {
	if r == nil || fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onAdvanced = append(r.onAdvanced, fn)
}

func (r *registry) NotifyAdvanced(event Advanced) {
	if r == nil || event.Store == nil {
		return
	}
	r.mu.RLock()
	callbacks := append([]func(Advanced){}, r.onAdvanced...)
	r.mu.RUnlock()
	for _, fn := range callbacks {
		if fn != nil {
			fn(event)
		}
	}
}
