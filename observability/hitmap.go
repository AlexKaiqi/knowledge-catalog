package observability

import "kc/knowledge"

// HitmapEntry is an aggregate of resolved access events. It is not a
// retrieval candidate or retrieval.KnowledgeHit.
type HitmapEntry struct {
	KnowledgeRef    knowledge.PinnedKnowledgeRef `json:"knowledgeRef"`
	Address         *knowledge.Address           `json:"address,omitempty"`
	Hits            int                          `json:"hits"`
	FirstAccessedAt string                       `json:"firstAccessedAt"`
	LastAccessedAt  string                       `json:"lastAccessedAt"`
	Principals      map[string]int               `json:"principals"`
	OnBehalfOf      map[string]int               `json:"onBehalfOf,omitempty"`
}
