package home

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

const managedLedgerFile = "managed.db"

var managedRecordsBucket = []byte("allocations-v1")
var managedCommandsBucket = []byte("commands-v1")

const (
	managedReserved   = "RESERVED"
	managedOwned      = "OWNED"
	managedRegistered = "REGISTERED"
	managedReady      = "READY"
)

type managedRecord struct {
	Request             ManagedRepositoryRequest `json:"request"`
	Digest              kernel.Digest            `json:"digest"`
	AllocationID        string                   `json:"allocationId"`
	Binding             RepositoryBinding        `json:"binding"`
	BackendID           string                   `json:"backendId,omitempty"`
	Head                kernel.CommitID          `json:"head,omitempty"`
	Actions             []string                 `json:"actions"`
	ShareActions        []string                 `json:"shareActions,omitempty"`
	Phase               string                   `json:"phase"`
	Name                string                   `json:"name,omitempty"`
	Owner               string                   `json:"owner,omitempty"`
	Store               string                   `json:"store,omitempty"`
	ManagementURL       string                   `json:"managementURL,omitempty"`
	ManagementState     string                   `json:"managementState,omitempty"`
	ProviderURL         string                   `json:"providerURL,omitempty"`
	AccountAllocationID string                   `json:"accountAllocationId,omitempty"`
	AccountBackendID    string                   `json:"accountBackendId,omitempty"`
	AccountEmailDomain  string                   `json:"accountEmailDomain,omitempty"`
	AccountAuthSourceID int64                    `json:"accountAuthSourceId,omitempty"`
}

func managedRank(phase string) int {
	switch phase {
	case managedReserved:
		return 1
	case managedOwned:
		return 2
	case managedRegistered:
		return 3
	case managedReady:
		return 4
	}
	return 0
}

func createManagedLedger(path string) error {
	if _, err := os.Stat(path); err == nil {
		records, readErr := loadManagedRecords(filepath.Dir(path))
		if readErr != nil {
			return readErr
		}
		if len(records) != 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "unrecognized managed allocations require their deployment initialization receipt")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(managedRecordsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(managedCommandsBucket)
		return err
	})
}

// Each operation owns a short Bolt transaction. Backend calls and grant
// callbacks never hold the database lock needed by deployment validation.
func managedDB(dir string, write bool, fn func(*bolt.Tx) error) error {
	path := filepath.Join(dir, managedLedgerFile)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository ledger is unavailable; restore durable state")
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: !write, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	check := func(tx *bolt.Tx) error {
		if tx.Bucket(managedRecordsBucket) == nil || tx.Bucket(managedCommandsBucket) == nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository ledger schema is missing")
		}
		return fn(tx)
	}
	if write {
		return db.Update(check)
	}
	return db.View(check)
}

func decodeManagedRecord(raw []byte) (managedRecord, error) {
	var record managedRecord
	if json.Unmarshal(raw, &record) != nil || record.AllocationID == "" || record.Request.RepositoryID == "" || record.Request.CatalogID == "" || record.Request.CommandID == "" || record.Request.Principal == "" || record.Binding.ID != record.Request.RepositoryID || record.Digest != kernel.CanonicalDigest(record.Request) || managedRank(record.Phase) == 0 || validateCreatorActions(record.Actions) != nil || validateShareActions(record.ShareActions) != nil {
		return record, kernel.Fail(kernel.ErrPreconditionFailed, "managed repository ledger contains an invalid allocation")
	}
	if _, err := hex.DecodeString(record.AllocationID); err != nil || len(record.AllocationID) != 32 {
		return record, kernel.Fail(kernel.ErrPreconditionFailed, "managed allocation identity is invalid")
	}
	if record.Phase != managedReserved && (record.BackendID == "" || record.Head == "") {
		return record, kernel.Fail(kernel.ErrPreconditionFailed, "managed source receipt is incomplete")
	}
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil || driver.managedOpen == nil {
		return record, kernel.Fail(kernel.ErrPreconditionFailed, "managed source driver is unavailable")
	}
	return record, nil
}

func managedCommandKey(req ManagedRepositoryRequest) []byte {
	return []byte(kernel.CanonicalDigest(struct{ Principal, CommandID string }{req.Principal, req.CommandID}))
}

func loadManagedRecords(dir string) ([]managedRecord, error) {
	var records []managedRecord
	err := managedDB(dir, false, func(tx *bolt.Tx) error {
		if err := tx.Bucket(managedRecordsBucket).ForEach(func(key, raw []byte) error {
			record, err := decodeManagedRecord(raw)
			if err != nil {
				return err
			}
			if string(key) != record.Request.RepositoryID || string(tx.Bucket(managedCommandsBucket).Get(managedCommandKey(record.Request))) != record.Request.RepositoryID {
				return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository indexes are inconsistent")
			}
			records = append(records, record)
			return nil
		}); err != nil {
			return err
		}
		return tx.Bucket(managedCommandsBucket).ForEach(func(key, value []byte) error {
			record, err := decodeManagedRecord(tx.Bucket(managedRecordsBucket).Get(value))
			if err != nil || string(key) != string(managedCommandKey(record.Request)) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "managed command index contains an invalid reference")
			}
			return nil
		})
	})
	return records, err
}

func reserveManaged(c DeploymentConfig, req ManagedRepositoryRequest, validateNew func() error) (managedRecord, error) {
	var record managedRecord
	err := managedDB(c.StateDir, true, func(tx *bolt.Tx) error {
		records, commands := tx.Bucket(managedRecordsBucket), tx.Bucket(managedCommandsBucket)
		if existing := commands.Get(managedCommandKey(req)); existing != nil {
			var err error
			record, err = decodeManagedRecord(records.Get(existing))
			if err != nil {
				return err
			}
			if record.Digest != kernel.CanonicalDigest(req) {
				return kernel.Fail(kernel.ErrIdempotencyConflict, "managed command was already reserved with another request")
			}
			return nil
		}
		if records.Get([]byte(req.RepositoryID)) != nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "repository identity already has a managed allocation")
		}
		pool, store, err := selectManagedPool(c, req.Store)
		if err != nil {
			return err
		}
		if err := validateNew(); err != nil {
			return err
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		allocation := hex.EncodeToString(nonce[:])
		driver, err := authorityFor(pool.Driver)
		if err != nil {
			return err
		}
		binding, err := driver.managedBinding(pool, req, allocation)
		if err != nil {
			return err
		}
		managementURL := driver.managedURL(pool, binding)
		if req.Name != "" && managementURL == "" {
			return kernel.Fail(kernel.ErrPreconditionFailed, "this managed store requires a public management URL")
		}
		record = managedRecord{Request: req, Digest: kernel.CanonicalDigest(req), AllocationID: allocation, Binding: binding, Actions: append([]string(nil), pool.CreatorActions...), ShareActions: append([]string(nil), pool.ShareActions...), Phase: managedReserved, Name: req.Name, Owner: req.Principal, Store: store, ManagementURL: managementURL}
		if record.ManagementURL != "" {
			record.ManagementState = "READY"
		}
		if validManagedUsername(req.Principal) && driver.managedEnsureOwner != nil {
			record.ProviderURL = binding.DSN
			if pool.PublicURL == "" && pool.AuthSourceID == 0 && !driver.managedTrustedOwner(req, binding) {
				record.ManagementState = "LOGIN_CONFIGURATION_REQUIRED"
			}
			record.AccountAllocationID, record.AccountEmailDomain, record.AccountAuthSourceID = allocation, pool.UserEmailDomain, pool.AuthSourceID
			// An account allocation is shared by this user's repositories in
			// this exact provider namespace, including an interrupted first repo.
			if err := records.ForEach(func(_, raw []byte) error {
				existing, err := decodeManagedRecord(raw)
				if err != nil {
					return err
				}
				if existing.Owner == record.Owner && existing.Binding.Driver == binding.Driver && existing.AccountAllocationID != "" && strings.TrimSuffix(existing.Binding.DSN, "/"+filepath.Base(existing.Binding.DSN)) == strings.TrimSuffix(binding.DSN, "/"+filepath.Base(binding.DSN)) {
					record.AccountAllocationID, record.AccountBackendID = existing.AccountAllocationID, existing.AccountBackendID
				}
				return nil
			}); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if err := records.Put([]byte(req.RepositoryID), raw); err != nil {
			return err
		}
		return commands.Put(managedCommandKey(req), []byte(req.RepositoryID))
	})
	return record, err
}

func saveManaged(dir string, record managedRecord) error {
	return managedDB(dir, true, func(tx *bolt.Tx) error {
		bucket := tx.Bucket(managedRecordsBucket)
		old, err := decodeManagedRecord(bucket.Get([]byte(record.Request.RepositoryID)))
		if err != nil {
			return err
		}
		if old.AllocationID != record.AllocationID || old.Digest != record.Digest || old.AccountAllocationID != record.AccountAllocationID || (old.AccountBackendID != "" && old.AccountBackendID != record.AccountBackendID) || old.ManagementURL != record.ManagementURL || (old.BackendID != "" && (old.BackendID != record.BackendID || old.Head != record.Head)) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "managed allocation changed during provisioning")
		}
		if managedRank(old.Phase) > managedRank(record.Phase) {
			return nil
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(record.Request.RepositoryID), raw)
	})
}
