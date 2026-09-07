package snapshot

import "kc/kernel"

// Advanced is a pure layer ⓪ ref movement. Changed knowledge identities are a
// layer ② concern and intentionally do not appear in this event.
type Advanced struct {
	Store Store
	From  kernel.CommitID
	To    kernel.CommitID
}

func (r *Registry) OnAdvanced(fn func(Advanced)) {
	if r == nil || fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onAdvanced = append(r.onAdvanced, fn)
}

func (r *Registry) NotifyAdvanced(event Advanced) {
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
