package snapshot

import "kc/kernel"

// Receipt is the protocol-owned write receipt (CAP-ORG-01 B9). It mirrors the
// JSON shape the Writer has always emitted, but lives at the capability root
// so callers (the invocation seam, product envelopes, hook events) never name
// a tree-writer implementation type. treewriter.Receipt stays unchanged; this
// type is additive, not a replacement.
type Receipt struct {
	ReceiptRef  string        `json:"receiptRef"`
	CommandID   string        `json:"commandId"`
	Surface     string        `json:"surface"`
	Disposition string        `json:"disposition"`
	Result      ReceiptResult `json:"result"`
}

// ReceiptResult carries the applied write coordinates.
type ReceiptResult struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
	TargetRef    string              `json:"targetRef"`
	OldCommit    kernel.CommitID     `json:"oldCommit"`
	NewCommit    kernel.CommitID     `json:"newCommit"`
}

// Receipt dispositions, mirroring the writer's vocabulary.
const (
	ReceiptApplied  = "APPLIED"
	ReceiptReplayed = "REPLAYED"
)
