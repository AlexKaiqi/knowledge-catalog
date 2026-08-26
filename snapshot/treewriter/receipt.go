package treewriter

import "kc/kernel"

type ReceiptDisposition string

const (
	DispositionApplied  ReceiptDisposition = "APPLIED"
	DispositionReplayed ReceiptDisposition = "REPLAYED"
)

type Receipt struct {
	ReceiptRef  string             `json:"receiptRef"`
	CommandID   string             `json:"commandId"`
	Surface     string             `json:"surface"`
	Disposition ReceiptDisposition `json:"disposition"`
	Result      Result             `json:"result"`
}

type Result struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
	TargetRef    string              `json:"targetRef"`
	OldCommit    kernel.CommitID     `json:"oldCommit"`
	NewCommit    kernel.CommitID     `json:"newCommit"`
}
