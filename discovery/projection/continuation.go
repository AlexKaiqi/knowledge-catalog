package projection

import "context"

// ContinuationSealer authenticates opaque execution cursors. It owns no query
// permission or resource lease. Prepared handles perform only local crypto.
type ContinuationSealer interface {
	SealContinuation([]byte) string
	OpenContinuation(string) ([]byte, error)
}

// ContinuationSealStore separates first-use preparation from existing-cursor
// verification. Loading must never replace missing or unknown signing material.
type ContinuationSealStore interface {
	PrepareContinuationSealer(context.Context) (ContinuationSealer, error)
	LoadContinuationSealer(context.Context) (ContinuationSealer, error)
}
