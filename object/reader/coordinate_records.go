package reader

import (
	"context"
	"errors"
	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"os"
	"path/filepath"
	"time"
)

// coordinateRecords stores only immutable, owner-validated read metadata.
// Each owner has its own bucket and record codec; queries never create files.
type coordinateRecords struct {
	path   string
	bucket []byte
}

func (s coordinateRecords) put(ctx context.Context, key string, raw []byte, validate func([]byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db, err := openCoordinateRecords(ctx, s.path, false)
	if err != nil {
		return coordinateStorageError(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket, err := tx.CreateBucketIfNotExists(s.bucket)
		if err != nil {
			return err
		}
		if previous := bucket.Get([]byte(key)); previous != nil {
			return validate(previous)
		}
		return bucket.Put([]byte(key), raw)
	})
	closeErr := db.Close()
	if err != nil {
		return coordinateStorageError(err)
	}
	if closeErr != nil {
		return coordinateStorageError(closeErr)
	}
	directory, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return coordinateStorageError(err)
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err != nil {
		return coordinateStorageError(err)
	}
	if closeErr != nil {
		return coordinateStorageError(closeErr)
	}
	return nil
}

func (s coordinateRecords) get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db, err := openCoordinateRecords(ctx, s.path, true)
	if os.IsNotExist(err) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "fixed read metadata record is unavailable")
	}
	if err != nil {
		return nil, coordinateStorageError(err)
	}
	var raw []byte
	err = db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket(s.bucket)
		if bucket == nil || bucket.Get([]byte(key)) == nil {
			return kernel.Fail(kernel.ErrVersionUnresolved, "fixed read metadata record is unavailable")
		}
		raw = append([]byte(nil), bucket.Get([]byte(key))...)
		return nil
	})
	closeErr := db.Close()
	if err != nil {
		return nil, coordinateStorageError(err)
	}
	if closeErr != nil {
		return nil, coordinateStorageError(closeErr)
	}
	return raw, nil
}

func openCoordinateRecords(ctx context.Context, path string, readOnly bool) (*bolt.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := time.Second
	deadline, bounded := ctx.Deadline()
	if bounded {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: readOnly, Timeout: timeout})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if bounded && !time.Now().Before(deadline) {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func coordinateStorageError(err error) error {
	if kernel.CodeOf(err) != "" || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "fixed read metadata storage is unavailable: %v", err)
}
