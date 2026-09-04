package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"kc/knowledge"
)

var stateValueBucket = []byte("values")

// stateRecord is the basis-addressable Serving State for one object. It is
// kept outside the Repository and outside the retrieval provider so search
// hits can be hydrated without retaining the whole knowledge space in memory.
type stateRecord struct {
	Value        knowledge.KnowledgeValue    `json:"value"`
	Observations []knowledge.UnitObservation `json:"observations,omitempty"`
	Doc          CompiledDoc                 `json:"doc"`
}

func (s *stateServingStore) WalkRecords(batchSize int, visit func(map[knowledge.ObjectID]stateRecord) error) error {
	if batchSize <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "Serving State batch size must be positive")
	}
	return s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		batch := make(map[knowledge.ObjectID]stateRecord, batchSize)
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			var record stateRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			batch[knowledge.ObjectID(key)] = record
			if len(batch) == batchSize {
				if err := visit(batch); err != nil {
					return err
				}
				batch = make(map[knowledge.ObjectID]stateRecord, batchSize)
			}
		}
		if len(batch) > 0 {
			return visit(batch)
		}
		return nil
	})
}

func (s *stateServingStore) WalkDocs(batchSize int, visit func([]CompiledDoc) error) error {
	if batchSize <= 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "Serving State batch size must be positive")
	}
	return s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		batch := make([]CompiledDoc, 0, batchSize)
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
	hash := sha256.New()
	_, _ = hash.Write([]byte(commit))
	err := s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateValueBucket).Cursor()
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
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
	db   *bolt.DB
	file string
	dir  string
	temp string
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
	identity := kernel.CanonicalDigest(map[string]any{"repository": key.repo, "commit": key.commit, "lane": key.lane})
	dir := filepath.Join(base, "state-serving", SanitizeID(string(key.repo))+"-"+string(identity)[:16])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if temp != "" {
			_ = os.RemoveAll(temp)
		}
		return nil, err
	}
	if revision == "building" {
		revision += "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	file := filepath.Join(dir, revision+".db")
	db, err := bolt.Open(file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		_ = os.Remove(dir)
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
		return nil, err
	}
	return store, nil
}

func (s *stateServingStore) PutBatch(records map[knowledge.ObjectID]stateRecord) error {
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

func (s *stateServingStore) Get(objectID knowledge.ObjectID) (stateRecord, bool, error) {
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

func (s *stateServingStore) CloseAndRemove() error {
	if s == nil {
		return nil
	}
	closeErr := s.db.Close()
	removeErr := os.Remove(s.file)
	_ = os.Remove(s.dir)
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
