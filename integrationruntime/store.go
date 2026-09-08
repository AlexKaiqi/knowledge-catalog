package integrationruntime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

var statesBucket = []byte("integrations-v1")
var artifactsBucket = []byte("artifacts-v1")

func decode(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func (r *Runtime) transaction(write bool, action func(*bolt.Tx) error) error {
	if _, err := os.Stat(filepath.Join(r.Directory, "runtime.db")); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "integration runtime ledger is unavailable; restore durable state")
	}
	db, err := bolt.Open(filepath.Join(r.Directory, "runtime.db"), 0600, &bolt.Options{ReadOnly: !write, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	checked := func(tx *bolt.Tx) error {
		if tx.Bucket(statesBucket) == nil || tx.Bucket(artifactsBucket) == nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "integration runtime ledger schema is unavailable; restore durable state")
		}
		return action(tx)
	}
	if write {
		return db.Update(checked)
	}
	return db.View(checked)
}

func (r *Runtime) load(id string) (state, error) {
	var value state
	err := r.transaction(false, func(tx *bolt.Tx) error {
		raw := tx.Bucket(statesBucket).Get([]byte(id))
		if raw == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "unknown integration %q", id)
		}
		if err := decode(raw, &value); err != nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "integration state is invalid; restore durable state")
		}
		return nil
	})
	return value, err
}

func (r *Runtime) edit(id string, action func(*state) error) error {
	return r.transaction(true, func(tx *bolt.Tx) error {
		bucket := tx.Bucket(statesBucket)
		var value state
		raw := bucket.Get([]byte(id))
		if raw == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "unknown integration %q", id)
		}
		if err := decode(raw, &value); err != nil {
			return err
		}
		if err := action(&value); err != nil {
			return err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(id), raw)
	})
}

func (r *Runtime) artifact(id string) (Artifact, error) {
	var artifact Artifact
	err := r.transaction(false, func(tx *bolt.Tx) error {
		raw := tx.Bucket(artifactsBucket).Get([]byte(id))
		if raw == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "build artifact %q before activation", id)
		}
		return decode(raw, &artifact)
	})
	return artifact, err
}

func initialize(directory string) error {
	if !filepath.IsAbs(directory) {
		return kernel.Fail(kernel.ErrUsageInvalid, "runtime state directory must be absolute")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	_, markerErr := os.Stat(filepath.Join(directory, "initialized"))
	_, dbErr := os.Stat(filepath.Join(directory, "runtime.db"))
	if markerErr == nil && os.IsNotExist(dbErr) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "integration runtime ledger is missing; restore durable state")
	}
	if os.IsNotExist(markerErr) && dbErr == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "integration runtime initialization receipt is missing; restore durable state")
	}
	db, err := bolt.Open(filepath.Join(directory, "runtime.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	if markerErr == nil {
		return db.View(func(tx *bolt.Tx) error {
			if tx.Bucket(statesBucket) == nil || tx.Bucket(artifactsBucket) == nil {
				return kernel.Fail(kernel.ErrPreconditionFailed, "integration runtime ledger schema is missing; restore durable state")
			}
			return nil
		})
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{statesBucket, artifactsBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "initialized"), []byte("integration-runtime/v1\n"), 0600)
}
