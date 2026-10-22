package projection

import (
	"context"
	"time"
)

// ObservationBinder admits one bounded read of an already published observation.
// An empty continuation selects the current observation once. An existing native
// continuation restores its protected generation and original expiry; it never
// falls back to the current pointer. Close releases only this request's lease.
// Dataset authorization and lifecycle eligibility remain the caller's duty.
type ObservationBinder interface {
	BindObservation(context.Context, string) (Engine, error)
}

// ObservationReference is one opaque accepted resource identity. A zero cutoff
// denotes the current observation; older identities admit continuations only
// before their original cutoff, independently of an admitted request's lifetime.
type ObservationReference struct {
	Evidence          []byte
	ContinuationUntil time.Time
}

// PublicationContinuationDeadlineBinder applies the authority's original
// observation cutoff inside resource query admission. It never shortens a query
// already admitted before that cutoff, and never extends the native cursor lease.
type PublicationContinuationDeadlineBinder interface {
	BindPublicationContinuationUntil(context.Context, []byte, string, time.Time) (Engine, error)
}
