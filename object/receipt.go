package object

import "kc/kernel"

// ReceiptDisposition describes the durable outcome of an Object write.
type ReceiptDisposition string

const (
	DispositionApplied  ReceiptDisposition = "APPLIED"
	DispositionReplayed ReceiptDisposition = "REPLAYED"
)

// CommitReceipt is shared by COMMIT and PROPOSAL and can be consumed without
// importing a Writer implementation. Its wire shape is the existing contract.
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
