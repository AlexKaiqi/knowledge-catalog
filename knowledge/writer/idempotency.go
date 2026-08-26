package writer

import (
	"encoding/json"

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
	Kind      string               `json:"kind"`
	ChangeSet *knowledge.ChangeSet `json:"changeSet,omitempty"`
}

func (w *Writer) Lookup(commandID string) (IdempotencyEntry, bool) {
	entry, ok := w.commands.Lookup(commandID)
	if !ok || len(entry.Request.ChangeSet) == 0 {
		return IdempotencyEntry{}, false
	}
	var receipt CommitReceipt
	var cs knowledge.ChangeSet
	if json.Unmarshal(entry.Receipt, &receipt) != nil || json.Unmarshal(entry.Request.ChangeSet, &cs) != nil {
		return IdempotencyEntry{}, false
	}
	return IdempotencyEntry{
		CommandID: entry.CommandID,
		Digest:    entry.Digest,
		Receipt:   receipt,
		Request:   WriterRequest{Kind: entry.Request.Kind, ChangeSet: &cs},
	}, true
}
