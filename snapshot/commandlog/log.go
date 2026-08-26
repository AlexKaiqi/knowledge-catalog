// Package commandlog provides the process-wide durable command-id ledger.
//
// It owns only replay/conflict mechanics. Surface-specific request and receipt
// semantics stay with their callers (for example knowledge/writer and
// snapshot/treewriter), which lets unrelated write surfaces share one command
// namespace without importing each other.
package commandlog

import (
	"encoding/json"
	"sync"

	"kc/kernel"
)

type Request struct {
	Kind          string          `json:"kind"`
	ChangeSet     json.RawMessage `json:"changeSet,omitempty"`
	TreeChangeSet json.RawMessage `json:"rawChangeSet,omitempty"`
}

type Entry struct {
	CommandID string          `json:"commandId"`
	Digest    string          `json:"digest"`
	Receipt   json.RawMessage `json:"receipt"`
	Request   Request         `json:"request,omitempty"`
}

type Store interface {
	Load() ([]Entry, error)
	Save([]Entry) error
}

// Ledger serializes check/apply/remember per command-id. Distinct commands may
// apply concurrently; entry mutation and durable saves remain synchronized.
type Ledger struct {
	mu      sync.Mutex
	store   Store
	entries map[string]Entry
	locks   map[string]*commandLock
}

type commandLock struct {
	mu    sync.Mutex
	users int
}

func New(store Store) (*Ledger, error) {
	l := &Ledger{store: store, entries: map[string]Entry{}, locks: map[string]*commandLock{}}
	if store == nil {
		return l, nil
	}
	entries, err := store.Load()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		l.entries[entry.CommandID] = entry
	}
	return l, nil
}

func (l *Ledger) Lookup(commandID string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[commandID]
	return entry, ok
}

func (l *Ledger) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.entriesLocked()
}

// Execute returns the stored entry with replayed=true when commandID already
// owns the same digest. A different digest fails before apply is called. Calls
// sharing a commandID serialize; unrelated command IDs may apply concurrently.
func (l *Ledger) Execute(commandID, digest string, request Request, apply func() (any, error)) (entry Entry, replayed bool, err error) {
	unlockCommand := l.lockCommand(commandID)
	defer unlockCommand()

	l.mu.Lock()
	if prior, ok := l.entries[commandID]; ok {
		l.mu.Unlock()
		if prior.Digest != digest {
			return Entry{}, false, kernel.Fail(kernel.ErrIdempotencyConflict,
				"command %s reused with different payload", commandID)
		}
		if len(prior.Receipt) == 0 {
			return Entry{}, false, kernel.Fail(kernel.ErrPreconditionFailed,
				"command %s has an unresolved prior outcome; inspect the target ref before retrying", commandID)
		}
		return prior, true, nil
	}
	// Persist the command claim before touching the authoritative repository.
	// If the process dies after apply but before the receipt is saved, restart
	// sees this pending entry and fails closed instead of applying twice.
	if l.store != nil {
		pending := Entry{CommandID: commandID, Digest: digest, Request: request}
		l.entries[commandID] = pending
		if err := l.store.Save(l.entriesLocked()); err != nil {
			delete(l.entries, commandID)
			l.mu.Unlock()
			return Entry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"reserve command %s in idempotency ledger: %v", commandID, err)
		}
	}
	l.mu.Unlock()
	receipt, err := apply()
	if err != nil {
		if l.store != nil {
			l.mu.Lock()
			delete(l.entries, commandID)
			_ = l.store.Save(l.entriesLocked())
			l.mu.Unlock()
		}
		return Entry{}, false, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return Entry{}, false, err
	}
	entry = Entry{CommandID: commandID, Digest: digest, Receipt: raw, Request: request}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[commandID] = entry
	if l.store != nil {
		if err := l.store.Save(l.entriesLocked()); err != nil {
			return Entry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"persist command %s receipt: %v", commandID, err)
		}
	}
	return entry, false, nil
}

// lockCommand keeps the per-command critical section while it has owners or
// waiters, then removes it. A long-running service therefore does not retain
// one mutex for every command_id it has ever seen.
func (l *Ledger) lockCommand(commandID string) func() {
	l.mu.Lock()
	lock := l.locks[commandID]
	if lock == nil {
		lock = &commandLock{}
		l.locks[commandID] = lock
	}
	lock.users++
	l.mu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		l.mu.Lock()
		defer l.mu.Unlock()
		lock.users--
		if lock.users == 0 && l.locks[commandID] == lock {
			delete(l.locks, commandID)
		}
	}
}

func (l *Ledger) entriesLocked() []Entry {
	out := make([]Entry, 0, len(l.entries))
	for _, entry := range l.entries {
		out = append(out, entry)
	}
	return out
}
