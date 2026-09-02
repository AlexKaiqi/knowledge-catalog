// Package observability defines non-canonical evidence produced when principals
// access versioned knowledge.
package observability

import "context"

// Receipt is the durable delivery acknowledgement for one evidence event.
// EvidenceID is assigned by the Recorder after the event has crossed the
// durability boundary; callers must not supply it.
type Receipt struct {
	EvidenceID string
}

// Recorder is the fail-closed write port. A successful knowledge response may
// be delivered only after the required evidence kinds have been acknowledged.
type Recorder interface {
	RecordAccess(context.Context, AccessEvent) (Receipt, error)
	RecordRetrieval(context.Context, RetrievalEvent) (Receipt, error)
	RecordRefine(context.Context, RefineEvent) (Receipt, error)
	RecordFeedback(context.Context, FeedbackEvent) error
}

// AccessLog is the audit read port for access evidence. GetAccess must observe
// an event after the Recorder that produced its Receipt acknowledged it.
type AccessLog interface {
	GetAccess(ctx context.Context, evidenceID string) (AccessEvent, bool, error)
	Access(ctx context.Context, query AccessQuery) (AccessPage, error)
}
