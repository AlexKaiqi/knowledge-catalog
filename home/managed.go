package home

import (
	"path/filepath"
	"strings"
	"sync"

	"kc/home/config"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type ManagedRepositoryRequest struct {
	RepositoryID     string
	CommandID        string
	Principal        string
	Name             string `json:"name,omitempty"`
	Store            string `json:"store,omitempty"`
	IdentityProvider string `json:"identityProvider,omitempty"`
	IdentitySubject  string `json:"identitySubject,omitempty"`
	IdentityIssuer   string `json:"identityIssuer,omitempty"`
}

type RepositoryInitialGrant struct {
	AllocationID string
	Principal    string
	RepositoryID string
	Actions      []string
}

// ManagedRepositoryGrant is retained for existing provisioning callers.
type ManagedRepositoryGrant = RepositoryInitialGrant

type ManagedRepositoryResult struct {
	RepositoryID      string          `json:"repositoryId"`
	CommandID         string          `json:"commandId"`
	Status            string          `json:"status"`
	Head              kernel.CommitID `json:"head"`
	Name              string          `json:"name,omitempty"`
	Owner             string          `json:"owner"`
	Store             string          `json:"store"`
	ManagementURL     string          `json:"managementURL"`
	ManagementState   string          `json:"managementState,omitempty"`
	ProviderURL       string          `json:"providerURL,omitempty"`
	ProvisioningState string          `json:"provisioningState"`
}

var managedOperationLocks sync.Map

// CreateManagedRepository resumes the same durable allocation after failures.
// The grant callback must be idempotent by AllocationID because a crash can
// occur after grant acceptance but before the managed receipt. READY replay
// never calls it again, even when initial permissions have been revoked.
func (ws *Home) CreateManagedRepository(req ManagedRepositoryRequest, grant func(ManagedRepositoryGrant) error) (ManagedRepositoryResult, error) {
	var result ManagedRepositoryResult
	if ws == nil || ws.Deployment == nil || ws.readOnly {
		return result, kernel.Fail(kernel.ErrPreconditionFailed, "managed provisioning requires a writable deployment")
	}
	id, err := NormalizeRepositoryID(req.RepositoryID)
	if err != nil || id != req.RepositoryID || strings.TrimSpace(req.CommandID) == "" || strings.TrimSpace(req.Principal) == "" || req.RepositoryID == string(objectmodel.SystemRepositoryID) {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "managed create requires a valid repository, commandId, and principal")
	}
	{
		binding := ws.Deployment.Registry
		if binding.ID == req.RepositoryID {
			return result, kernel.Fail(kernel.ErrTargetRepositoryDenied, "registry authority cannot be provisioned as a member Snapshot")
		}
	}
	value, _ := managedOperationLocks.LoadOrStore(filepath.Clean(ws.Dir), &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := ValidateDeploymentState(*ws.Deployment); err != nil {
		return result, err
	}
	if err := ws.Registry.CheckAuthority(); err != nil {
		return result, err
	}
	record, err := reserveManaged(*ws.Deployment, req, func() error {
		if grant == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "managed create requires the configured creator-policy callback")
		}
		if _, exists := ws.Store.Get(kernel.RepositoryID(req.RepositoryID)); exists {
			return kernel.Fail(kernel.ErrPreconditionFailed, "repository identity already has a source binding")
		}
		{
			existing := ws.Datasets
			if existing.HasRepository(kernel.RepositoryID(req.RepositoryID)) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "repository identity is already admitted")
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result = managedResult(record)
	result.Status = "APPLIED"
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil {
		return result, err
	}
	var source snapshot.Store
	if record.Phase == managedReserved {
		pool, _, err := selectManagedPool(*ws.Deployment, record.Store)
		if err != nil {
			return result, err
		}
		source, record.BackendID, err = driver.managedCreate(pool, record.Binding, record.AllocationID)
		if err != nil {
			return result, err
		}
		record.Head, err = source.Head(snapshot.DefaultRef)
		if err != nil {
			closeManagedSource(source)
			return result, err
		}
		record.Phase = managedOwned
		if err = saveManaged(ws.Dir, record); err != nil {
			closeManagedSource(source)
			return result, err
		}
	} else {
		source, err = driver.managedOpen(record.Binding, record.AllocationID, record.BackendID)
		if err != nil {
			return result, err
		}
		if !source.HasCommit(record.Head) {
			closeManagedSource(source)
			return result, kernel.Fail(kernel.ErrPreconditionFailed, "managed initial published commit is unavailable")
		}
	}
	if err := ws.installManagedSource(record, source); err != nil {
		closeManagedSource(source)
		return result, err
	}
	result.Head = record.Head
	result.ProvisioningState = record.Phase
	if record.Phase == managedReady {
		result.Status = "REPLAYED"
		return result, nil
	}
	if grant == nil {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "managed retry requires the creator-policy callback")
	}
	if err := ws.Registry.CheckAuthority(); err != nil {
		return result, err
	}
	if err := grant(ManagedRepositoryGrant{AllocationID: record.AllocationID, Principal: req.Principal, RepositoryID: req.RepositoryID, Actions: append([]string(nil), record.Actions...)}); err != nil {
		return result, err
	}
	record.Phase = managedReady
	if err := saveManaged(ws.Dir, record); err != nil {
		return result, err
	}
	result.ProvisioningState = managedReady
	return result, nil
}

func closeManagedSource(source snapshot.Store) {
	if closer, ok := source.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func (ws *Home) installManagedSource(record managedRecord, source snapshot.Store) error {
	if _, ok := ws.Store.Get(source.ID()); ok {
		closeManagedSource(source)
	} else if err := ws.Store.Add(source); err != nil {
		return err
	}
	ws.inventory.mu.Lock()
	defer ws.inventory.mu.Unlock()
	ws.inventory.bindings[record.Binding.ID] = repositoryHomeRepo(record.Binding)
	return nil
}

func validateManagedBindings(c DeploymentConfig, records []managedRecord) error {
	bindings := c
	bindings.ManagedRepositories = nil
	bindings.ManagedStores = nil
	bindings.Repositories = append([]RepositoryBinding(nil), c.Repositories...)
	for _, record := range records {
		if record.legacyAuthority != "" && record.legacyAuthority != c.Registry.ID {
			return kernel.Fail(kernel.ErrPreconditionFailed, "managed allocation belongs to a different registry authority")
		}
		bindings.Repositories = append(bindings.Repositories, record.Binding)
	}
	// Validate historical bindings even after new provisioning is disabled.
	// In particular a replacement cache location cannot encompass authority.
	return bindings.Validate()
}

func (ws *Home) verifyManagedBinding(id kernel.RepositoryID) error {
	if err := ValidateDeploymentState(*ws.Deployment); err != nil {
		return err
	}
	records, err := loadManagedRecords(ws.Dir)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Binding.ID != string(id) || record.Phase == managedReserved {
			continue
		}
		driver, err := authorityFor(record.Binding.Driver)
		if err != nil {
			return err
		}
		source, err := driver.managedOpen(record.Binding, record.AllocationID, record.BackendID)
		if err != nil {
			return err
		}
		defer closeManagedSource(source)
		if !source.HasCommit(record.Head) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "managed initial published commit is unavailable")
		}
		return nil
	}
	return kernel.Fail(kernel.ErrPreconditionFailed, "repository %s has no provisioned managed binding", id)
}

func validateCreatorActions(actions []string) error {
	return config.ValidateCreatorActions(actions)
}

func validateShareActions(actions []string) error {
	return config.ValidateShareActions(actions)
}

func validManagedUsername(value string) bool {
	return config.ValidManagedUsername(value)
}
