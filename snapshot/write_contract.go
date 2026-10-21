package snapshot

import "kc/snapshot/internal/storecontract"

// WriteObserver receives the actual outcome for external evidence recording.
// It cannot erase an existing publication error. If publication succeeded,
// its error is returned alongside the accepted receipt; the command remains
// APPLIED and a retry replays the receipt without another publication.
type WriteObserver func(surface string, refs map[string]any, err error) error

// RawWriter publishes literal path bytes with CAS and durable idempotency.
type RawWriter interface {
	SetObserver(WriteObserver)
	SetStamp(author, requestID, ruleID string)
	// ForRequest shares the authority and command ledger and owns only one
	// request's observer and stamp.
	ForRequest(observer WriteObserver, author, requestID, ruleID string) RawWriter
	Lookup(string) (TreeChangeSet, Receipt, bool)
	// Commit may return an APPLIED or REPLAYED receipt with an observer error.
	// Such an error does not mean the authoritative write was unapplied.
	Commit(string, TreeChangeSet) (Receipt, error)
}

// RawWriteSurface is the durable literal-file write surface stamp.
const RawWriteSurface = "RAW_WRITE"

// WriteNotApplied marks an error only when the caller can prove that the
// authoritative write did not publish. CommandLedger may release that claim
// for retry. Other errors retain PENDING until explicitly resolved or abandoned.
// The wrapper preserves the original error and its protocol code via Unwrap.
func WriteNotApplied(err error) error { return storecontract.WriteNotApplied(err) }
