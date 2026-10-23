// Package boltfile opens the bbolt databases this repository keeps as durable
// ledgers. Those ledgers share two properties that belong here rather than in
// each caller.
//
// First, a process must never race itself on one file. bbolt takes an
// operating-system lock for the whole time a database handle is open, so a
// ledger that opens per operation serializes against its own concurrent
// readers and writers. Callers therefore keep their per-operation open/close
// shape, and this package shares one open handle per path inside the process:
// a reader reuses whatever handle is already open instead of queueing behind
// another reader. A writer needs the file to itself, so it waits for the shares
// to drain and then reopens the file for writing. While a writer waits, new
// readers queue behind it, so continuous reads cannot starve a write. Every
// wait is bounded by the caller's own open timeout, and a wait that outlasts it
// answers TEMPORARY_UNAVAILABLE: the same request is safe to retry.
//
// Second, a lock that could not be taken is a transient condition, not a
// request defect. bbolt reports it as ErrTimeout, whose message is the bare
// word "timeout"; a caller that lets it escape is normalized into
// USAGE_INVALID and tells a client to change a request that was well formed.
// Every open here classifies that outcome as TEMPORARY_UNAVAILABLE.
package boltfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"

	"kc/kernel"
)

// defaultTimeout matches the wait bbolt itself defaults to when a caller does
// not name one.
const defaultTimeout = time.Second

// pollInterval bounds how long a waiting caller sleeps before it rechecks the
// shared entry. Reuse is the common case, so the interval only shapes how
// quickly a queued caller notices that the state it waited for changed.
const pollInterval = time.Millisecond

// Handle is one caller's share of this process's handle for a file. The
// embedded database is the caller's whole interface; Close releases this share
// instead of closing a database another caller still uses.
type Handle struct {
	*bolt.DB
	entry *entry

	once sync.Once
	err  error
}

func (h *Handle) Close() error {
	if h == nil || h.DB == nil {
		return nil
	}
	h.once.Do(func() { h.err = h.entry.release(h.DB) })
	return h.err
}

// entry is this process's share of one file. It outlives individual handles so
// that a later caller reuses the same path instead of racing a fresh open, and
// so that every waiter observes one state transition.
type entry struct {
	key string

	mu            sync.Mutex
	db            *bolt.DB
	refs          int
	opening       bool
	waiters       int
	writerWaiting bool
}

var (
	entriesMu sync.Mutex
	entries   = map[string]*entry{}
)

// entryFor returns this process's entry for path. An entry is forgotten once
// nothing references or waits on it, so a service that opens one file per
// observation revision does not retain an entry for every revision it has
// served.
func entryFor(path string) *entry {
	key, err := filepath.Abs(path)
	if err != nil {
		key = path
	}
	entriesMu.Lock()
	defer entriesMu.Unlock()
	found := entries[key]
	if found == nil {
		found = &entry{key: key}
		entries[key] = found
	}
	return found
}

// forget drops an entry that no share, waiter, or open references anymore.
func (e *entry) forget() {
	e.mu.Lock()
	idle := e.refs == 0 && e.waiters == 0 && !e.opening && e.db == nil
	e.mu.Unlock()
	if !idle {
		return
	}
	entriesMu.Lock()
	defer entriesMu.Unlock()
	if entries[e.key] == e {
		delete(entries, e.key)
	}
}

// Open returns a handle for path, reusing this process's open handle when it
// can serve the request. The handle must be closed; the database itself closes
// when its last share is released.
func Open(path string, mode os.FileMode, options *bolt.Options) (*Handle, error) {
	timeout := defaultTimeout
	if options != nil && options.Timeout > 0 {
		timeout = options.Timeout
	}
	write := options == nil || !options.ReadOnly
	deadline := time.Now().Add(timeout)
	shared := entryFor(path)
	for {
		shared.mu.Lock()
		// A reader reuses whatever handle is already open, including a writer's:
		// bbolt allows concurrent readers within one process. A writer needs the
		// file to itself, so it waits until no share remains and then opens the
		// file for writing. Readers queue behind a waiting writer so that a
		// steady read stream cannot starve it.
		if !write && shared.db != nil && !shared.writerWaiting {
			shared.refs++
			db := shared.db
			shared.mu.Unlock()
			return &Handle{DB: db, entry: shared}, nil
		}
		if shared.db == nil && !shared.opening {
			shared.opening = true
			shared.mu.Unlock()
			db, err := bolt.Open(path, mode, options)
			shared.mu.Lock()
			shared.opening = false
			if err != nil {
				shared.mu.Unlock()
				shared.forget()
				return nil, classify(path, err)
			}
			if shared.refs == 0 {
				shared.db, shared.refs = db, 1
				shared.mu.Unlock()
				return &Handle{DB: db, entry: shared}, nil
			}
			// Another caller opened the same file first. Keep its handle so the
			// process still holds exactly one, and try again against it.
			shared.mu.Unlock()
			_ = db.Close()
			continue
		}
		if write {
			shared.writerWaiting = true
		}
		shared.waiters++
		shared.mu.Unlock()
		expired := !time.Now().Before(deadline)
		time.Sleep(pollInterval)
		shared.mu.Lock()
		shared.waiters--
		if write {
			shared.writerWaiting = false
		}
		shared.mu.Unlock()
		shared.forget()
		if expired {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"%s is being reopened for writing by another operation in this process; retry the same request", filepath.Base(path))
		}
	}
}

// release drops one share. The database closes with the last one, so a reader
// that is still working keeps the file protected from a writer that has not
// started yet.
func (e *entry) release(db *bolt.DB) error {
	e.mu.Lock()
	if e.db != db {
		e.mu.Unlock()
		return nil
	}
	e.refs--
	if e.refs > 0 {
		e.mu.Unlock()
		return nil
	}
	err := e.db.Close()
	e.db = nil
	e.mu.Unlock()
	e.forget()
	return err
}

// classify keeps the lock timeout a transient condition and leaves every other
// open failure as it is: a missing directory or a corrupt file is not something
// the same request can retry into success.
func classify(path string, err error) error {
	if errors.Is(err, bolterrors.ErrTimeout) {
		return kernel.Fail(kernel.ErrTemporaryUnavailable,
			"%s is locked by another process; retry the same request", filepath.Base(path))
	}
	return err
}
