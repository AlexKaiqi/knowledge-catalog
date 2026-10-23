package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	ports "kc/discovery/projection"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/internal/boltfile"
	"kc/kernel"
	objectmodel "kc/object"
)

var stateValueBucket = []byte("values")

// stateRecord is the basis-addressable Serving State for one object. It is
// kept outside the Repository and outside the retrieval provider so search
// hits can be hydrated without retaining the whole object space in memory.
type stateRecord struct {
	Value        objectmodel.ObjectValue       `json:"value"`
	Observations []objectmodel.UnitObservation `json:"observations,omitempty"`
	Doc          ports.CompiledDoc             `json:"doc"`
}

func (s *stateServingStore) WalkRecords(batchSize int, visit func(map[objectmodel.ObjectID]stateRecord) error) error {
	if batchSize <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "Serving State batch size must be positive")
	}
	return s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		batch := make(map[objectmodel.ObjectID]stateRecord, batchSize)
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			var record stateRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			batch[objectmodel.ObjectID(key)] = record
			if len(batch) == batchSize {
				if err := visit(batch); err != nil {
					return err
				}
				batch = make(map[objectmodel.ObjectID]stateRecord, batchSize)
			}
		}
		if len(batch) > 0 {
			return visit(batch)
		}
		return nil
	})
}

func (s *stateServingStore) WalkDocs(batchSize int, visit func([]ports.CompiledDoc) error) error {
	if batchSize <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "Serving State batch size must be positive")
	}
	return s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		batch := make([]ports.CompiledDoc, 0, batchSize)
		for _, raw := cursor.First(); raw != nil; _, raw = cursor.Next() {
			var record stateRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			batch = append(batch, record.Doc)
			if len(batch) == batchSize {
				if err := visit(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		if len(batch) > 0 {
			return visit(batch)
		}
		return nil
	})
}

func (s *stateServingStore) Digest(commit kernel.CommitID) (kernel.Digest, error) {
	return s.DigestContext(context.Background(), commit)
}

func (s *stateServingStore) DigestContext(ctx context.Context, commit kernel.CommitID) (kernel.Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(commit))
	err := s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, _ = hash.Write(key)
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write(raw)
			_, _ = hash.Write([]byte{0})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return kernel.Digest(hex.EncodeToString(hash.Sum(nil))), nil
}

type stateServingStore struct {
	db        *boltfile.Handle
	file      string
	dir       string
	temp      string
	closeOnce sync.Once
	closeErr  error
	sealed    bool
}

func openStateServingStore(root string, key engineKey, revision string) (*stateServingStore, error) {
	base := root
	temp := ""
	var err error
	if base == "" {
		base, err = os.MkdirTemp("", "kc-serving-state-")
		if err != nil {
			return nil, err
		}
		temp = base
	}
	dir := stateServingDirectory(base, key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	var candidate *os.File
	if revision == "building" {
		candidate, err = os.CreateTemp(dir, "building-*.db")
	} else {
		candidate, err = os.OpenFile(filepath.Join(dir, revision+".db"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	file := candidate.Name()
	if err := candidate.Close(); err != nil {
		_ = os.Remove(file)
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	db, err := boltfile.Open(file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		_ = os.Remove(file)
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	store := &stateServingStore{db: db, file: file, dir: dir, temp: temp}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(stateValueBucket)
		return err
	}); err != nil {
		_ = db.Close()
		_ = os.Remove(file)
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	return store, nil
}

func (s *stateServingStore) PutBatch(records map[objectmodel.ObjectID]stateRecord) error {
	if s.sealed {
		return kernel.Fail(kernel.ErrPreconditionFailed, "published observation values are immutable")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateValueBucket)
		for objectID, record := range records {
			raw, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(objectID), raw); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *stateServingStore) Get(objectID objectmodel.ObjectID) (stateRecord, bool, error) {
	var record stateRecord
	found := false
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(stateValueBucket).Get([]byte(objectID))
		if raw == nil {
			return nil
		}
		found = true
		return json.Unmarshal(raw, &record)
	})
	return record, found, err
}

func (s *stateServingStore) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() { s.closeErr = s.db.Close() })
	return s.closeErr
}

// release retains configured durable storage; anonymous storage has no owner
// after its Index closes and may be discarded.
func (s *stateServingStore) release() error {
	if s != nil && s.temp != "" {
		return s.CloseAndRemove()
	}
	return s.Close()
}

func (s *stateServingStore) CloseAndRemove() error {
	if s == nil {
		return nil
	}
	closeErr := s.Close()
	removeErr := os.Remove(s.file)
	if s.temp != "" {
		_ = os.RemoveAll(s.temp)
	}
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func stateStoreKey(repo kernel.RepositoryID, commit kernel.CommitID) engineKey {
	return engineKey{repo: repo, commit: commit, lane: "state"}
}
