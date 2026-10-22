package writer

import (
	"bytes"
	"encoding/json"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// IdempotencyEntry is the Object Writer view of the shared command ledger.
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
	if !ok || (entry.Request.Kind != string(objectmodel.SurfaceCommit) && entry.Request.Kind != mergeKind) {
		return IdempotencyEntry{}, false
	}
	var receipt CommitReceipt
	raw := bytes.TrimSpace(entry.Receipt)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		// Only the constrained no-change phase supplies a recoverable original
		// basis without a completion receipt. A zero receipt is not success.
		if entry.Status != snapshot.StatusPending || entry.Phase != snapshot.CommandPhaseNoChange || entry.Execution == "" {
			return IdempotencyEntry{}, false
		}
	} else if json.Unmarshal(raw, &receipt) != nil {
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
