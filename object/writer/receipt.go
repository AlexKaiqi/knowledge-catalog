package writer

import "kc/object"

// Durable Receipt: APPLIED on first success, REPLAYED on same command_id + digest.
// COMMIT and PROPOSAL share CommitReceipt.

type ReceiptDisposition = object.ReceiptDisposition

const (
	DispositionApplied  = object.DispositionApplied
	DispositionReplayed = object.DispositionReplayed
)

// CommitReceipt is the durable result of COMMIT or PROPOSAL (same snapshot apply).
type CommitReceipt = object.CommitReceipt

type CommitResult = object.CommitResult
