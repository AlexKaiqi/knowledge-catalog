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
	locks   map[string]*sync.Mutex
}

func New(store Store) (*Ledger, error) {
	l := &Ledger{store: store, entries: map[string]Entry{}, locks: map[string]*sync.Mutex{}}
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
	commandMu := l.commandMutex(commandID)
	commandMu.Lock()
	defer commandMu.Unlock()

	l.mu.Lock()
	if prior, ok := l.entries[commandID]; ok {
		l.mu.Unlock()
		if prior.Digest != digest {
			return Entry{}, false, kernel.Fail(kernel.ErrIdempotencyConflict,
				"command %s reused with different payload", commandID)
		}
		return prior, true, nil
	}
	l.mu.Unlock()
	receipt, err := apply()
	if err != nil {
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
			return Entry{}, false, err
		}
	}
	return entry, false, nil
}

func (l *Ledger) commandMutex(commandID string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	mu := l.locks[commandID]
	if mu == nil {
		mu = &sync.Mutex{}
		l.locks[commandID] = mu
	}
	return mu
}

func (l *Ledger) entriesLocked() []Entry {
	out := make([]Entry, 0, len(l.entries))
	for _, entry := range l.entries {
		out = append(out, entry)
	}
	return out
}
