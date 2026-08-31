package hook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"kc/internal/jsonfile"
)

var outboxLocks = struct {
	sync.Mutex
	items map[string]*outboxLock
}{items: map[string]*outboxLock{}}

type outboxLock struct {
	mu    sync.Mutex
	users int
}

type OutboxItem struct {
	Binding  Binding `json:"binding"`
	Event    Event   `json:"event"`
	Error    string  `json:"error,omitempty"`
	QueuedAt string  `json:"queuedAt,omitempty"`
}

type OutboxStats struct {
	Pending         int
	OldestPendingAt time.Time
}

func OutboxPath(home string) string { return filepath.Join(home, "hook-outbox.jsonl") }

func AppendOutbox(home string, b Binding, event Event, deliverErr error) error {
	return withOutboxLock(home, func() error {
		return appendOutbox(home, b, event, deliverErr)
	})
}

func appendOutbox(home string, b Binding, event Event, deliverErr error) error {
	item := OutboxItem{Binding: b, Event: event, QueuedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if deliverErr != nil {
		item.Error = deliverErr.Error()
	}
	return jsonfile.AppendJSONL(OutboxPath(home), item)
}

func FlushOutbox(home string) error {
	return FlushOutboxObserved(home, nil)
}

func FlushOutboxObserved(home string, observe DispatchObserver) error {
	return withOutboxLock(home, func() error { return flushOutbox(home, observe) })
}

func flushOutbox(home string, observe DispatchObserver) error {
	path := OutboxPath(home)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	// Decode line by line via json.Decoder on the whole file.
	remaining := []OutboxItem{}
	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		var item OutboxItem
		if err := dec.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		started := time.Now()
		deliveryErr := deliver(home, item.Binding, item.Event)
		observeDispatch(observe, PhasePost, "outbox", deliveryErr, time.Since(started))
		if deliveryErr != nil {
			item.Error = deliveryErr.Error()
			remaining = append(remaining, item)
		}
	}
	if len(remaining) == 0 {
		return os.Remove(path)
	}
	return replaceOutbox(path, remaining)
}

func InspectOutbox(home string) (OutboxStats, error) {
	var stats OutboxStats
	err := withOutboxLock(home, func() error {
		path := OutboxPath(home)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		for {
			var item OutboxItem
			if err := dec.Decode(&item); err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			stats.Pending++
			queuedAt, parseErr := time.Parse(time.RFC3339Nano, item.QueuedAt)
			if parseErr != nil {
				queuedAt = info.ModTime()
			}
			if stats.OldestPendingAt.IsZero() || queuedAt.Before(stats.OldestPendingAt) {
				stats.OldestPendingAt = queuedAt
			}
		}
		return nil
	})
	return stats, err
}

// withOutboxLock serializes one home's append/flush across goroutines and kc
// processes. A per-home process mutex avoids flock's same-process edge cases;
// the advisory file lock protects the remove/replace window between processes.
func withOutboxLock(home string, action func() error) error {
	lockPath := OutboxPath(home) + ".lock"
	unlockLocal := lockOutbox(lockPath)
	defer unlockLocal()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	unlockFile, err := lockFile(file)
	if err != nil {
		return err
	}
	defer unlockFile()
	return action()
}

func lockOutbox(path string) func() {
	outboxLocks.Lock()
	lock := outboxLocks.items[path]
	if lock == nil {
		lock = &outboxLock{}
		outboxLocks.items[path] = lock
	}
	lock.users++
	outboxLocks.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		outboxLocks.Lock()
		defer outboxLocks.Unlock()
		lock.users--
		if lock.users == 0 && outboxLocks.items[path] == lock {
			delete(outboxLocks.items, path)
		}
	}
}

// replaceOutbox keeps the previous durable file in place until the complete
// retry set has been written and synced. A crash may redeliver an event (post
// hooks are at-least-once), but it cannot erase undelivered events by landing
// between remove and append.
func replaceOutbox(path string, items []OutboxItem) error {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hook-outbox-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(body.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
