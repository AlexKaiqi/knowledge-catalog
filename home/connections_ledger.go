package home

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

const connectionLedgerFile = "connections.db"

var connectionRecordsBucket = []byte("bindings-v1")
var connectionCredentialsBucket = []byte("credentials-v1")

// Credentials are kept in a separate private bucket, never in public bindings.
type connectionRecord struct {
	AllocationID     string            `json:"allocationId"`
	Owner            string            `json:"owner"`
	Catalog          string            `json:"catalog"`
	Binding          RepositoryBinding `json:"binding"`
	BackendID        string            `json:"backendId"`
	InitialHead      kernel.CommitID   `json:"initialHead"`
	Actions          []string          `json:"actions"`
	ShareActions     []string          `json:"shareActions,omitempty"`
	Phase            string            `json:"phase"`
	Revision         uint64            `json:"revision"`
	CredentialDigest kernel.Digest     `json:"credentialDigest"`
}

func createConnectionLedger(path string) error {
	if _, err := os.Stat(path); err == nil {
		records, err := loadConnectionRecords(filepath.Dir(path))
		if err != nil {
			return err
		}
		if len(records) > 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "connection initialization receipt is missing")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{connectionRecordsBucket, connectionCredentialsBucket} {
			if _, err := tx.CreateBucket(name); err != nil {
				return err
			}
		}
		return nil
	})
}

func connectionDB(dir string, write bool, fn func(*bolt.Tx) error) error {
	path := filepath.Join(dir, connectionLedgerFile)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "private connection ledger is unavailable; restore durable state with mode 0600")
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: !write, Timeout: time.Second})
	if err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "private connection ledger cannot be opened")
	}
	defer db.Close()
	check := func(tx *bolt.Tx) error {
		if tx.Bucket(connectionRecordsBucket) == nil || tx.Bucket(connectionCredentialsBucket) == nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "private connection ledger schema is missing")
		}
		return fn(tx)
	}
	if write {
		return db.Update(check)
	}
	return db.View(check)
}

func decodeConnection(raw, secret []byte) (connectionRecord, error) {
	var r connectionRecord
	invalid := func() (connectionRecord, error) {
		return r, kernel.Fail(kernel.ErrPreconditionFailed, "private connection ledger contains an invalid binding")
	}
	if json.Unmarshal(raw, &r) != nil || len(r.AllocationID) != 32 || r.Owner == "" || r.Catalog == "" || r.Binding.ID == "" || r.Binding.Driver != "gitea" || r.Binding.Dir != "" || r.BackendID == "" || r.InitialHead == "" || r.Revision == 0 || (r.Phase != "CONNECTED" && r.Phase != "READY") || validateConnectionActions(r.Actions) != nil || validateShareActions(r.ShareActions) != nil || len(secret) == 0 || r.CredentialDigest != kernel.CanonicalDigest(string(secret)) {
		return invalid()
	}
	if _, err := connectionOrigin(r.Binding.DSN); err != nil {
		return invalid()
	}
	return r, nil
}

func loadConnectionRecords(dir string) ([]connectionRecord, error) {
	var records []connectionRecord
	err := connectionDB(dir, false, func(tx *bolt.Tx) error {
		bindings, secrets := tx.Bucket(connectionRecordsBucket), tx.Bucket(connectionCredentialsBucket)
		if err := bindings.ForEach(func(k, v []byte) error {
			r, err := decodeConnection(v, secrets.Get(k))
			if err != nil {
				return err
			}
			if r.Binding.ID != string(k) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "connection binding index is invalid")
			}
			records = append(records, r)
			return nil
		}); err != nil {
			return err
		}
		return secrets.ForEach(func(k, _ []byte) error {
			if bindings.Get(k) == nil {
				return kernel.Fail(kernel.ErrPreconditionFailed, "connection credential has no binding")
			}
			return nil
		})
	})
	return records, err
}

func readConnection(dir, id string) (connectionRecord, string, error) {
	var record connectionRecord
	var credential string
	err := connectionDB(dir, false, func(tx *bolt.Tx) error {
		raw := tx.Bucket(connectionRecordsBucket).Get([]byte(id))
		if raw == nil {
			return kernel.Fail(kernel.ErrTargetRepositoryDenied, "repository has no connection")
		}
		secret := tx.Bucket(connectionCredentialsBucket).Get([]byte(id))
		var err error
		record, err = decodeConnection(raw, secret)
		credential = string(secret)
		return err
	})
	return record, credential, err
}

func saveConnection(dir string, record *connectionRecord, credential string, expected uint64) error {
	return connectionDB(dir, true, func(tx *bolt.Tx) error {
		key := []byte(record.Binding.ID)
		records, secrets := tx.Bucket(connectionRecordsBucket), tx.Bucket(connectionCredentialsBucket)
		if raw := records.Get(key); raw != nil {
			current, err := decodeConnection(raw, secrets.Get(key))
			if err != nil {
				return err
			}
			if current.Revision != expected {
				return kernel.Fail(kernel.ErrPreconditionFailed, "connection changed; reload before retry")
			}
		} else if expected != 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "connection receipt disappeared")
		}
		record.Revision = expected + 1
		record.CredentialDigest = kernel.CanonicalDigest(credential)
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := decodeConnection(raw, []byte(credential)); err != nil {
			return err
		}
		if err := records.Put(key, raw); err != nil {
			return err
		}
		return secrets.Put(key, []byte(credential))
	})
}
