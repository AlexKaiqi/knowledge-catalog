package writer

import (
	"encoding/json"
	"os"

	"kc/internal/jsonfile"
	"kc/kernel"
	"kc/repository"
)

// command_id log: Lookup, conflictOrReplay, persist to .kc/writer.json.
// Same id + digest → original Receipt; same id + different digest → IDEMPOTENCY_CONFLICT.

type IdempotencyStore interface {
	Load() ([]IdempotencyEntry, error)
	Save(entries []IdempotencyEntry) error
}

type FileIdempotencyStore struct {
	file string
}

func NewFileIdempotencyStore(file string) *FileIdempotencyStore {
	return &FileIdempotencyStore{file: file}
}

func (s *FileIdempotencyStore) Load() ([]IdempotencyEntry, error) {
	if _, err := os.Stat(s.file); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []IdempotencyEntry
	if err := jsonfile.Read(s.file, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *FileIdempotencyStore) Save(entries []IdempotencyEntry) error {
	return jsonfile.Write(s.file, entries)
}

type WriterRequest struct {
	Kind      string                      `json:"kind"`
	ChangeSet *repository.CommitChangeSet `json:"changeSet,omitempty"`
	Entries   *repository.AppendEntries   `json:"entries,omitempty"`
}

type IdempotencyEntry struct {
	CommandID string        `json:"commandId"`
	Digest    string        `json:"digest"`
	Receipt   any           `json:"receipt"`
	Request   WriterRequest `json:"request,omitempty"`
}

type storedEntry struct {
	digest  string
	receipt any
	request WriterRequest
}

func (w *Writer) Lookup(commandID string) (IdempotencyEntry, bool) {
	entry, ok := w.idempotency[commandID]
	if !ok {
		return IdempotencyEntry{}, false
	}
	return IdempotencyEntry{CommandID: commandID, Digest: entry.digest, Receipt: entry.receipt, Request: entry.request}, true
}

func (w *Writer) conflictOrReplay(commandID, digest string) (any, bool, error) {
	prior, ok := w.idempotency[commandID]
	if !ok {
		return nil, false, nil
	}
	if prior.digest != digest {
		return nil, false, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s reused with different payload", commandID)
	}
	return prior.receipt, true, nil
}

func (w *Writer) DumpIdempotency() []IdempotencyEntry {
	out := make([]IdempotencyEntry, 0, len(w.idempotency))
	for commandID, entry := range w.idempotency {
		out = append(out, IdempotencyEntry{
			CommandID: commandID,
			Digest:    entry.digest,
			Receipt:   entry.receipt,
			Request:   entry.request,
		})
	}
	return out
}

func (w *Writer) LoadIdempotency(entries []IdempotencyEntry) {
	w.idempotency = map[string]storedEntry{}
	for _, entry := range entries {
		w.idempotency[entry.CommandID] = storedEntry{
			digest:  entry.Digest,
			receipt: decodeReceipt(entry.Request.Kind, entry.Receipt),
			request: entry.Request,
		}
	}
}

func (w *Writer) remember(commandID, digest string, receipt any, request WriterRequest) error {
	w.idempotency[commandID] = storedEntry{digest: digest, receipt: receipt, request: request}
	if w.log == nil {
		return nil
	}
	return w.log.Save(w.DumpIdempotency())
}

func decodeReceipt(kind string, raw any) any {
	if raw == nil {
		return nil
	}
	if _, ok := raw.(CommitReceipt); ok && (kind == string(repository.SurfaceCommit) || kind == string(repository.SurfaceProposal)) {
		return raw
	}
	if _, ok := raw.(AppendReceipt); ok && kind == string(repository.SurfaceAppend) {
		return raw
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return raw
	}
	if kind == string(repository.SurfaceAppend) {
		var receipt AppendReceipt
		if json.Unmarshal(b, &receipt) == nil {
			return receipt
		}
	}
	var receipt CommitReceipt
	if json.Unmarshal(b, &receipt) == nil {
		return receipt
	}
	return raw
}
