package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	index "kc/discovery/projection"
	"kc/internal/boltfile"
	"kc/kernel"
)

var projectionTargetBucket = []byte("targets")
var stateNoticeBucket = []byte("state-notices")

type TargetStore struct{ file string }

func NewTargetStore(file string) *TargetStore { return &TargetStore{file: file} }

func (s *TargetStore) ready() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(projectionTargetBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(stateNoticeBucket)
		return err
	})
}

func (s *TargetStore) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(projectionTargetBucket)
		if err != nil {
			return err
		}
		target := index.ProjectionTarget{Repository: repository}
		if raw := bucket.Get([]byte(repository)); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
		}
		target.DesiredCommit = commit
		target.Status = index.TargetPending
		target.LastError = ""
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), raw)
	})
}

func (s *TargetStore) List() ([]index.ProjectionTarget, error) {
	targets := []index.ProjectionTarget{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var target index.ProjectionTarget
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			targets = append(targets, target)
			return nil
		})
	})
	return targets, err
}

func (s *TargetStore) failHead(repository kernel.RepositoryID, fault error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			// The journal was reinitialized under a live worker. Recording the
			// fault is best effort: there is no target row to fail, and writing
			// one would invent a demand nobody asked for.
			return nil
		}
		target := index.ProjectionTarget{Repository: repository}
		if raw := bucket.Get([]byte(repository)); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
		}
		target.Status = index.TargetFailed
		target.LastError = fault.Error()
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), raw)
	})
}

func (s *TargetStore) finish(repository kernel.RepositoryID, attempted kernel.CommitID, syncResult index.IndexSync, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		raw := bucket.Get([]byte(repository))
		if raw == nil {
			return nil
		}
		var target index.ProjectionTarget
		if err := json.Unmarshal(raw, &target); err != nil {
			return err
		}
		if target.DesiredCommit != attempted {
			return nil
		}
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if applyErr != nil {
			target.Status = index.TargetFailed
			target.LastError = applyErr.Error()
		} else {
			target.Status = index.TargetReady
			target.LastError = ""
			target.AppliedCommit = syncResult.BasisCommit
		}
		next, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), next)
	})
}

func (s *TargetStore) view(fn func(*bolt.Tx) error) error {
	db, err := boltfile.Open(s.file, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}

func (s *TargetStore) update(fn func(*bolt.Tx) error) error {
	db, err := boltfile.Open(s.file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}
