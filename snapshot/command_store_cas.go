package snapshot

import (
	"encoding/json"
	"errors"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

func (s *boltCommandStore) CompareAndSwap(commandID string, expected, replacement *CommandEntry) (bool, error) {
	if commandID == "" || expected != nil && expected.CommandID != commandID || replacement != nil && replacement.CommandID != commandID {
		return false, errors.New("conditional command entry identity does not match")
	}
	matched := false
	err := s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(commandBucket)
		if err != nil {
			return err
		}
		raw := bucket.Get([]byte(commandID))
		var current *CommandEntry
		if raw != nil {
			current = &CommandEntry{}
			if err := json.Unmarshal(raw, current); err != nil {
				return err
			}
		}
		if !sameCommandEntry(current, expected) {
			return nil
		}
		if replacement == nil {
			if err := bucket.Delete([]byte(commandID)); err != nil {
				return err
			}
		} else {
			next, err := json.Marshal(replacement)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(commandID), next); err != nil {
				return err
			}
		}
		matched = true
		return nil
	})
	return matched, err
}

func sameCommandEntry(a, b *CommandEntry) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return kernel.CanonicalDigest(a) == kernel.CanonicalDigest(b)
}

// replaceCommand is called with this ledger's per-command lock held. A
// persistent conditional store also fences every other ledger instance.
func (l *commandLedger) replaceCommand(expected, replacement *CommandEntry) error {
	id := ""
	if expected != nil {
		id = expected.CommandID
	}
	if replacement != nil {
		id = replacement.CommandID
	}
	if store, ok := l.store.(ConditionalCommandStore); ok {
		matched, err := store.CompareAndSwap(id, expected, replacement)
		if err != nil {
			return err
		}
		if !matched {
			return kernel.Fail(kernel.ErrPreconditionFailed, "command %s execution was superseded", id)
		}
	} else if l.store != nil {
		var err error
		if replacement == nil {
			err = l.store.Delete(id)
		} else {
			err = l.store.Put(*replacement)
		}
		if err != nil {
			return err
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if replacement == nil {
		delete(l.entries, id)
	} else {
		l.entries[id] = *replacement
	}
	return nil
}
