package writer

import "kc/kernel"

// Durable Receipt: APPLIED on first success, REPLAYED on same command_id + digest.
// COMMIT and PROPOSAL share CommitReceipt.

type ReceiptDisposition string

const (
	DispositionApplied  ReceiptDisposition = "APPLIED"
	DispositionReplayed ReceiptDisposition = "REPLAYED"
)

// CommitReceipt is the durable result of COMMIT or PROPOSAL (same snapshot apply).
type CommitReceipt struct {
	ReceiptRef  string             `json:"receiptRef"`
	CommandID   string             `json:"commandId"`
	Surface     string             `json:"surface"`
	Disposition ReceiptDisposition `json:"disposition"`
	Result      CommitResult       `json:"result"`
}

type CommitResult struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
	TargetRef    string              `json:"targetRef"`
	OldCommit    kernel.CommitID     `json:"oldCommit"`
	NewCommit    kernel.CommitID     `json:"newCommit"`
}
