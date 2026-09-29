package observability

import "kc/kernel"

// TraceContext aliases the protocol-base correlation shape (B13 ruling A).
type TraceContext = kernel.TraceContext

type TraceEntry struct {
	Kind       string          `json:"kind"`
	OccurredAt string          `json:"occurredAt"`
	Access     *AccessEvent    `json:"access,omitempty"`
	Feedback   *FeedbackEvent  `json:"feedback,omitempty"`
	Retrieval  *RetrievalEvent `json:"retrieval,omitempty"`
	Refine     *RefineEvent    `json:"refine,omitempty"`
}

type TraceView struct {
	TraceID string       `json:"traceId"`
	Entries []TraceEntry `json:"entries"`
}
