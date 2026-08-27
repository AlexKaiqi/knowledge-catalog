package index

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"kc/knowledge"
)

var projectionTargetBucket = []byte("targets")

type ProjectionTarget struct {
	Repository    kernel.RepositoryID `json:"repository"`
	DesiredCommit kernel.CommitID     `json:"desiredCommit"`
	AppliedCommit kernel.CommitID     `json:"appliedCommit,omitempty"`
	Status        string              `json:"status"`
	LastError     string              `json:"lastError,omitempty"`
	UpdatedAt     string              `json:"updatedAt"`
}

const (
	TargetPending = "PENDING"
	TargetReady   = "READY"
	TargetFailed  = "FAILED"
)

type TargetStore struct{ file string }

func NewTargetStore(file string) *TargetStore { return &TargetStore{file: file} }

func (s *TargetStore) ready() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(projectionTargetBucket)
		return err
	})
}

func (s *TargetStore) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(projectionTargetBucket)
		if err != nil {
			return err
		}
		target := ProjectionTarget{Repository: repository}
		if raw := bucket.Get([]byte(repository)); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
		}
		target.DesiredCommit = commit
		target.Status = TargetPending
		target.LastError = ""
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), raw)
	})
}

func (s *TargetStore) List() ([]ProjectionTarget, error) {
	targets := []ProjectionTarget{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var target ProjectionTarget
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			targets = append(targets, target)
			return nil
		})
	})
	return targets, err
}

func (s *TargetStore) finish(repository kernel.RepositoryID, attempted kernel.CommitID, syncResult IndexSync, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		raw := bucket.Get([]byte(repository))
		if raw == nil {
			return nil
		}
		var target ProjectionTarget
		if err := json.Unmarshal(raw, &target); err != nil {
			return err
		}
		if target.DesiredCommit != attempted {
			return nil
		}
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if applyErr != nil {
			target.Status = TargetFailed
			target.LastError = applyErr.Error()
		} else {
			target.Status = TargetReady
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
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}

func (s *TargetStore) update(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}

type RepositoryLookup func(kernel.RepositoryID) (knowledge.Repository, error)

// Controller durably coalesces desired projection commits. Desire is the only
// work performed on the Writer receipt path; compilation and provider I/O run
// in the background or through explicit CatchUp.
type Controller struct {
	index  *Index
	store  *TargetStore
	lookup RepositoryLookup
	wake   chan struct{}
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewController(index *Index, store *TargetStore, lookup RepositoryLookup) (*Controller, error) {
	if err := store.ready(); err != nil {
		return nil, err
	}
	return &Controller{index: index, store: store, lookup: lookup, wake: make(chan struct{}, 1)}, nil
}

func (c *Controller) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	if err := c.store.Desire(repository, commit); err != nil {
		return err
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return nil
}

func (c *Controller) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		_ = c.CatchUp(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.wake:
				_ = c.CatchUp(ctx)
			}
		}
	}()
}

func (c *Controller) Close() {
	if c.cancel != nil {
		c.cancel()
		c.wg.Wait()
	}
}

func (c *Controller) CatchUp(ctx context.Context) error {
	targets, err := c.store.List()
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if target.Status == TargetReady && target.AppliedCommit == target.DesiredCommit {
			continue
		}
		repo, lookupErr := c.lookup(target.Repository)
		result := IndexSync{}
		applyErr := lookupErr
		if applyErr == nil {
			result, applyErr = c.index.Ensure(repo, target.DesiredCommit)
		}
		if err := c.store.finish(target.Repository, target.DesiredCommit, result, applyErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) Targets() ([]ProjectionTarget, error) { return c.store.List() }
