package snapshot

// WriteObserver receives the actual outcome for external evidence recording.
// It has no storage authority and cannot substitute a publication result.
type WriteObserver func(surface string, refs map[string]any, err error) error

// RawWriter publishes literal path bytes with CAS and durable idempotency.
type RawWriter interface {
	SetObserver(WriteObserver)
	SetStamp(author, requestID, ruleID string)
	Lookup(string) (TreeChangeSet, Receipt, bool)
	Commit(string, TreeChangeSet) (Receipt, error)
}

// RawWriteSurface is the durable literal-file write surface stamp.
const RawWriteSurface = "RAW_WRITE"
