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
	"time"

	"kc/kernel"
)

type Request struct {
	Kind                 string          `json:"kind"`
	RepositoryID         string          `json:"repositoryId,omitempty"`
	TargetRef            string          `json:"targetRef,omitempty"`
	BaseCommit           string          `json:"baseCommit,omitempty"`
	ExpectedTargetCommit string          `json:"expectedTargetCommit,omitempty"`
	OperationCount       int             `json:"operationCount,omitempty"`
	TreeChangeSet        json.RawMessage `json:"rawChangeSet,omitempty"`
}

type Entry struct {
	CommandID string          `json:"commandId"`
	Digest    string          `json:"digest"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
	Receipt   json.RawMessage `json:"receipt"`
	Request   Request         `json:"request,omitempty"`
}

const (
	StatusPending   = "PENDING"
	StatusApplied   = "APPLIED"
	StatusAbandoned = "ABANDONED"
)

type Store interface {
	Ready() error
	Get(commandID string) (Entry, bool, error)
	Put(Entry) error
	Delete(commandID string) error
	List() ([]Entry, error)
}

type retentionStore interface {
	PruneBefore(time.Time) (int, error)
}

// ResolvePending records the operator/provider conclusion for a command that
// crossed the authority boundary but lost its receipt write. The digest must
// still match the durable reservation; this method never reapplies work.
func (l *Ledger) ResolvePending(commandID, digest string, receipt any) (Entry, error) {
	unlockCommand := l.lockCommand(commandID)
	defer unlockCommand()
	prior, ok, lookupErr := l.lookup(commandID)
	if lookupErr != nil {
		return Entry{}, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"read pending command %s: %v", commandID, lookupErr)
	}
	if !ok || prior.Status != StatusPending || len(prior.Receipt) != 0 {
		return Entry{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"command %s has no unresolved pending reservation", commandID)
	}
	if prior.Digest != digest {
		return Entry{}, kernel.Fail(kernel.ErrIdempotencyConflict,
			"command %s pending digest does not match", commandID)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return Entry{}, err
	}
	prior.Status = StatusApplied
	prior.Receipt = raw
	prior.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if l.store != nil {
		if err := l.store.Put(prior); err != nil {
			return Entry{}, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"resolve pending command %s: %v", commandID, err)
		}
	}
	l.mu.Lock()
	l.entries[commandID] = prior
	l.mu.Unlock()
	return prior, nil
}

// AbandonPending records an explicit decision that a pending reservation must
// not be replayed. It remains auditable until the declared retention window
// expires and Prune removes it.
func (l *Ledger) AbandonPending(commandID, digest string) (Entry, error) {
	unlockCommand := l.lockCommand(commandID)
	defer unlockCommand()
	prior, ok, lookupErr := l.lookup(commandID)
	if lookupErr != nil {
		return Entry{}, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"read pending command %s: %v", commandID, lookupErr)
	}
	if !ok || prior.Status != StatusPending || len(prior.Receipt) != 0 {
		return Entry{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"command %s has no unresolved pending reservation", commandID)
	}
	if prior.Digest != digest {
		return Entry{}, kernel.Fail(kernel.ErrIdempotencyConflict,
			"command %s pending digest does not match", commandID)
	}
	prior.Status = StatusAbandoned
	prior.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if l.store != nil {
		if err := l.store.Put(prior); err != nil {
			return Entry{}, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"abandon pending command %s: %v", commandID, err)
		}
	}
	l.mu.Lock()
	l.entries[commandID] = prior
	l.mu.Unlock()
	return prior, nil
}

// Prune removes completed or explicitly abandoned entries older than before.
// Unresolved PENDING entries are never inferred or deleted.
func (l *Ledger) Prune(before time.Time) (int, error) {
	if store, ok := l.store.(retentionStore); ok {
		removed, err := store.PruneBefore(before)
		if err != nil {
			return 0, kernel.Fail(kernel.ErrTemporaryUnavailable, "prune command ledger: %v", err)
		}
		l.mu.Lock()
		for id, entry := range l.entries {
			updated, parseErr := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
			if parseErr == nil && updated.Before(before) &&
				(entry.Status == StatusApplied || entry.Status == StatusAbandoned) {
				delete(l.entries, id)
			}
		}
		l.mu.Unlock()
		return removed, nil
	}
	var entries []Entry
	if l.store != nil {
		var err error
		entries, err = l.store.List()
		if err != nil {
			return 0, kernel.Fail(kernel.ErrTemporaryUnavailable, "list command ledger for pruning: %v", err)
		}
	} else {
		l.mu.Lock()
		entries = l.entriesLocked()
		l.mu.Unlock()
	}
	removed := 0
	for _, candidate := range entries {
		if candidate.Status != StatusApplied && candidate.Status != StatusAbandoned {
			continue
		}
		updated, err := time.Parse(time.RFC3339Nano, candidate.UpdatedAt)
		if err != nil || !updated.Before(before) {
			continue
		}
		unlockCommand := l.lockCommand(candidate.CommandID)
		current, ok, lookupErr := l.lookup(candidate.CommandID)
		if lookupErr != nil {
			unlockCommand()
			return removed, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"read command %s while pruning: %v", candidate.CommandID, lookupErr)
		}
		if ok && current.Digest == candidate.Digest && current.UpdatedAt == candidate.UpdatedAt &&
			(current.Status == StatusApplied || current.Status == StatusAbandoned) {
			if l.store != nil {
				if err := l.store.Delete(candidate.CommandID); err != nil {
					unlockCommand()
					return removed, kernel.Fail(kernel.ErrTemporaryUnavailable,
						"prune command %s: %v", candidate.CommandID, err)
				}
			}
			l.mu.Lock()
			delete(l.entries, candidate.CommandID)
			l.mu.Unlock()
			removed++
		}
		unlockCommand()
	}
	return removed, nil
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
	if err := store.Ready(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Ledger) Lookup(commandID string) (Entry, bool) {
	entry, ok, _ := l.lookup(commandID)
	return entry, ok
}

func (l *Ledger) lookup(commandID string) (Entry, bool, error) {
	l.mu.Lock()
	entry, ok := l.entries[commandID]
	l.mu.Unlock()
	if ok || l.store == nil {
		return entry, ok, nil
	}
	entry, ok, err := l.store.Get(commandID)
	if err != nil || !ok {
		return Entry{}, false, err
	}
	l.mu.Lock()
	l.entries[commandID] = entry
	l.mu.Unlock()
	return entry, true, nil
}

func (l *Ledger) Entries() []Entry {
	if l.store != nil {
		entries, err := l.store.List()
		if err == nil {
			return entries
		}
	}
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

	prior, ok, lookupErr := l.lookup(commandID)
	if lookupErr != nil {
		return Entry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"read command %s from idempotency ledger: %v", commandID, lookupErr)
	}
	if ok {
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Persist the command claim before touching the authoritative repository.
	// If the process dies after apply but before the receipt is saved, restart
	// sees this pending entry and fails closed instead of applying twice.
	pending := Entry{CommandID: commandID, Digest: digest, Status: StatusPending, CreatedAt: now, UpdatedAt: now, Request: request}
	if l.store != nil {
		if err := l.store.Put(pending); err != nil {
			return Entry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"reserve command %s in idempotency ledger: %v", commandID, err)
		}
	}
	l.mu.Lock()
	l.entries[commandID] = pending
	l.mu.Unlock()
	receipt, err := apply()
	if err != nil {
		if l.store != nil {
			_ = l.store.Delete(commandID)
		}
		l.mu.Lock()
		delete(l.entries, commandID)
		l.mu.Unlock()
		return Entry{}, false, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return Entry{}, false, err
	}
	entry = Entry{CommandID: commandID, Digest: digest, Status: StatusApplied, CreatedAt: now,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), Receipt: raw, Request: request}
	if l.store != nil {
		if err := l.store.Put(entry); err != nil {
			return Entry{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"persist command %s receipt: %v", commandID, err)
		}
	}
	l.mu.Lock()
	l.entries[commandID] = entry
	l.mu.Unlock()
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
