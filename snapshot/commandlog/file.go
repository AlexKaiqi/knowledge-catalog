package commandlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

var commandBucket = []byte("commands")

// BoltStore persists one command per key. It opens the database per operation
// so short-lived CLI homes do not leak file locks; a service deployment can
// put the same Store contract behind a shared control database.
type BoltStore struct {
	existing bool
	file     string
}

func NewBoltStore(file string) *BoltStore { return &BoltStore{file: file} }

// NewFileStore remains source compatible, but now uses keyed bbolt storage.
func NewFileStore(file string) *BoltStore { return NewBoltStore(file) }

func (s *BoltStore) Ready() error {
	if s.existing {
		return s.checkExisting()
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(commandBucket)
		return err
	})
}

func (s *BoltStore) Get(commandID string) (Entry, bool, error) {
	var entry Entry
	found := false
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		raw := bucket.Get([]byte(commandID))
		if raw == nil {
			return nil
		}
		found = true
		return json.Unmarshal(raw, &entry)
	})
	return entry, found, err
}

func (s *BoltStore) Put(entry Entry) error {
	if entry.CommandID == "" {
		return errors.New("command id is required")
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(commandBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(entry.CommandID), raw)
	})
}

func (s *BoltStore) Delete(commandID string) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(commandID))
	})
}

func (s *BoltStore) List() ([]Entry, error) {
	entries := []Entry{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var entry Entry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return err
			}
			entries = append(entries, entry)
			return nil
		})
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].CommandID < entries[j].CommandID })
	return entries, err
}

// PruneBefore deletes completed entries in one Bolt transaction without
// materializing the command history. PENDING entries are never inferred.
func (s *BoltStore) PruneBefore(before time.Time) (int, error) {
	removed := 0
	err := s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			var entry Entry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return err
			}
			if entry.Status != StatusApplied && entry.Status != StatusAbandoned {
				continue
			}
			updated, err := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
			if err != nil || !updated.Before(before) {
				continue
			}
			if err := cursor.Delete(); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	return removed, err
}

func (s *BoltStore) view(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}

func (s *BoltStore) update(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}

// OpenBoltStore reopens a durable ledger without initializing, migrating, or
// repairing it. A missing database or command bucket is a recovery failure.
func OpenBoltStore(file string) (*BoltStore, error) {
	s := &BoltStore{file: file, existing: true}
	if err := s.checkExisting(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *BoltStore) checkExisting() error {
	info, err := os.Stat(s.file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("command ledger is not a regular file")
	}
	return s.view(func(tx *bolt.Tx) error {
		if tx.Bucket(commandBucket) == nil {
			return fmt.Errorf("durable command ledger bucket is missing")
		}
		return nil
	})
}
