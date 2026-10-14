package observability

import objectmodel "kc/object"

// HitmapEntry is an aggregate of resolved access events. It is not a
// retrieval candidate or retrieval.ObjectHit.
type HitmapEntry struct {
	ObjectRef       objectmodel.PinnedObjectRef `json:"objectRef"`
	Address         *objectmodel.Address        `json:"address,omitempty"`
	Hits            int                         `json:"hits"`
	FirstAccessedAt string                      `json:"firstAccessedAt"`
	LastAccessedAt  string                      `json:"lastAccessedAt"`
	Principals      map[string]int              `json:"principals"`
	OnBehalfOf      map[string]int              `json:"onBehalfOf,omitempty"`
}
