package snapshot

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

// boltCommandStore persists one command per key. It opens the database per operation
// so short-lived CLI homes do not leak file locks; a service deployment can
// put the same CommandStore contract behind a shared control database.
type boltCommandStore struct {
	existing bool
	file     string
}

func NewBoltCommandStore(file string) CommandStore { return &boltCommandStore{file: file} }

func (s *boltCommandStore) Ready() error {
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

func (s *boltCommandStore) Get(commandID string) (CommandEntry, bool, error) {
	var entry CommandEntry
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

func (s *boltCommandStore) Put(entry CommandEntry) error {
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

func (s *boltCommandStore) Delete(commandID string) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(commandID))
	})
}

func (s *boltCommandStore) List() ([]CommandEntry, error) {
	entries := []CommandEntry{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var entry CommandEntry
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
func (s *boltCommandStore) PruneBefore(before time.Time) (int, error) {
	removed := 0
	err := s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(commandBucket)
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			var entry CommandEntry
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

func (s *boltCommandStore) view(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}

func (s *boltCommandStore) update(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}

// OpenBoltCommandStore reopens a durable ledger without initializing, migrating, or
// repairing it. A missing database or command bucket is a recovery failure.
func OpenBoltCommandStore(file string) (CommandStore, error) {
	s := &boltCommandStore{file: file, existing: true}
	if err := s.checkExisting(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *boltCommandStore) checkExisting() error {
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
