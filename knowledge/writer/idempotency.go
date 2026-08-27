package writer

import (
	"encoding/json"

	"kc/kernel"
	"kc/knowledge"
)

// IdempotencyEntry is the Knowledge Writer view of the shared command ledger.
// Raw tree requests deliberately do not appear in this package.
type IdempotencyEntry struct {
	CommandID string        `json:"commandId"`
	Digest    string        `json:"digest"`
	Receipt   CommitReceipt `json:"receipt"`
	Request   WriterRequest `json:"request,omitempty"`
}

type WriterRequest struct {
	Kind                 string              `json:"kind"`
	RepositoryID         kernel.RepositoryID `json:"repositoryId,omitempty"`
	TargetRef            string              `json:"targetRef,omitempty"`
	BaseCommit           kernel.CommitID     `json:"baseCommit,omitempty"`
	ExpectedTargetCommit kernel.CommitID     `json:"expectedTargetCommit,omitempty"`
	OperationCount       int                 `json:"operationCount,omitempty"`
}

func (w *Writer) Lookup(commandID string) (IdempotencyEntry, bool) {
	entry, ok := w.commands.Lookup(commandID)
	if !ok || (entry.Request.Kind != string(knowledge.SurfaceCommit) && entry.Request.Kind != string(knowledge.SurfaceProposal)) {
		return IdempotencyEntry{}, false
	}
	var receipt CommitReceipt
	if json.Unmarshal(entry.Receipt, &receipt) != nil {
		return IdempotencyEntry{}, false
	}
	return IdempotencyEntry{
		CommandID: entry.CommandID,
		Digest:    entry.Digest,
		Receipt:   receipt,
		Request: WriterRequest{
			Kind: entry.Request.Kind, RepositoryID: kernel.RepositoryID(entry.Request.RepositoryID), TargetRef: entry.Request.TargetRef,
			BaseCommit: kernel.CommitID(entry.Request.BaseCommit), ExpectedTargetCommit: kernel.CommitID(entry.Request.ExpectedTargetCommit),
			OperationCount: entry.Request.OperationCount,
		},
	}, true
}
