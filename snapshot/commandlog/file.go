package commandlog

import (
	"encoding/json"
	"errors"
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
	file   string
	legacy string
}

func NewBoltStore(file string, legacyJSON ...string) *BoltStore {
	store := &BoltStore{file: file}
	if len(legacyJSON) > 0 {
		store.legacy = legacyJSON[0]
	}
	return store
}

// NewFileStore remains source compatible, but now uses keyed bbolt storage.
func NewFileStore(file string) *BoltStore { return NewBoltStore(file) }

func (s *BoltStore) Ready() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(commandBucket)
		if err != nil || bucket.Stats().KeyN != 0 || s.legacy == "" {
			return err
		}
		raw, err := os.ReadFile(s.legacy)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var legacy []struct {
			CommandID string          `json:"commandId"`
			Digest    string          `json:"digest"`
			Receipt   json.RawMessage `json:"receipt"`
			Request   struct {
				Kind          string          `json:"kind"`
				ChangeSet     json.RawMessage `json:"changeSet"`
				TreeChangeSet json.RawMessage `json:"rawChangeSet"`
			} `json:"request"`
		}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return err
		}
		for _, old := range legacy {
			entry := Entry{
				CommandID: old.CommandID, Digest: old.Digest, Receipt: old.Receipt,
				Request: Request{Kind: old.Request.Kind, TreeChangeSet: old.Request.TreeChangeSet},
			}
			var basis struct {
				TargetRepository     string            `json:"targetRepository"`
				TargetRef            string            `json:"targetRef"`
				BaseCommit           string            `json:"baseCommit"`
				ExpectedTargetCommit string            `json:"expectedTargetCommit"`
				Operations           []json.RawMessage `json:"operations"`
				Changes              []json.RawMessage `json:"changes"`
			}
			requestRaw := old.Request.ChangeSet
			if len(requestRaw) == 0 {
				requestRaw = old.Request.TreeChangeSet
			}
			if len(requestRaw) > 0 {
				if err := json.Unmarshal(requestRaw, &basis); err != nil {
					return err
				}
				entry.Request.RepositoryID = basis.TargetRepository
				entry.Request.TargetRef = basis.TargetRef
				entry.Request.BaseCommit = basis.BaseCommit
				entry.Request.ExpectedTargetCommit = basis.ExpectedTargetCommit
				entry.Request.OperationCount = len(basis.Operations)
				if entry.Request.OperationCount == 0 {
					entry.Request.OperationCount = len(basis.Changes)
				}
			}
			if entry.CommandID == "" {
				continue
			}
			if entry.Status == "" {
				if len(entry.Receipt) == 0 {
					entry.Status = StatusPending
				} else {
					entry.Status = StatusApplied
				}
			}
			encoded, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(entry.CommandID), encoded); err != nil {
				return err
			}
		}
		return nil
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
