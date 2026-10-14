package maintenance

import (
	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"os"
	"path/filepath"
	"time"
)

type ProgressTarget struct {
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

type ProgressStore struct{ file string }

func (s *ProgressStore) ready() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error { _, err := tx.CreateBucketIfNotExists(consumerTargetBucket); return err })
}
func (s *ProgressStore) update(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}
func (s *ProgressStore) view(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}
