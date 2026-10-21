package writer

import "kc/internal/journal"

// ForRequest returns a Writer for one request. It shares the Snapshot
// registry and command ledger — the write authority, whose expected-commit
// checks and per-command serialization already decide concurrent writes — and
// owns only that request's journal and stamp. Concurrent requests therefore
// never overwrite each other's evidence and need no process-wide lock.
func (w *Writer) ForRequest(j journal.Journal, as, requestID, ruleID string) *Writer {
	if w == nil {
		return nil
	}
	return &Writer{
		store:         w.store,
		commands:      w.commands,
		journal:       j,
		author:        as,
		requestID:     requestID,
		ruleID:        ruleID,
		systemPublish: w.systemPublish,
	}
}
