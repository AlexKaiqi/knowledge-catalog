package treewriter

import (
	"kc/kernel"
	"kc/snapshot"
)

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

// Protocol converts the writer receipt to the protocol-owned snapshot.Receipt
// (CAP-ORG-01 B9: callers consume the capability-root type, not this package).
func (r Receipt) Protocol() snapshot.Receipt {
	return snapshot.Receipt{
		ReceiptRef:  r.ReceiptRef,
		CommandID:   r.CommandID,
		Surface:     r.Surface,
		Disposition: string(r.Disposition),
		Result: snapshot.ReceiptResult{
			RepositoryID: r.Result.RepositoryID,
			CommitID:     r.Result.CommitID,
			TargetRef:    r.Result.TargetRef,
			OldCommit:    r.Result.OldCommit,
			NewCommit:    r.Result.NewCommit,
		},
	}
}
