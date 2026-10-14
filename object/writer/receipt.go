package writer

import "kc/object"

// Durable Receipt: APPLIED on first success, REPLAYED on same command_id + digest.
// COMMIT and MERGE share CommitReceipt.

type ReceiptDisposition = object.ReceiptDisposition

const (
	DispositionApplied  = object.DispositionApplied
	DispositionReplayed = object.DispositionReplayed
)

// CommitReceipt is the durable result of COMMIT (snapshot apply) or MERGE (fast-forward).
type CommitReceipt = object.CommitReceipt

type CommitResult = object.CommitResult
