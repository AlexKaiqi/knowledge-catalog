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
	r.onAdvanced = append(r.onAdvanced, fn)
}

func (r *Registry) NotifyAdvanced(event Advanced) {
	if r == nil || event.Store == nil {
		return
	}
	for _, fn := range r.onAdvanced {
		if fn != nil {
			fn(event)
		}
	}
}
