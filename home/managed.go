package home

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// ManagedRepositoryConfig delegates allocation in one deployment-owned pool.
// It is not a Repository binding; individual allocations live in durable state.
type ManagedRepositoryConfig struct {
	Driver          string   `json:"driver" yaml:"driver"`
	Root            string   `json:"root,omitempty" yaml:"root,omitempty"`
	DSN             string   `json:"dsn,omitempty" yaml:"dsn,omitempty"`
	PublicURL       string   `json:"publicURL,omitempty" yaml:"publicURL,omitempty"`
	UserEmailDomain string   `json:"userEmailDomain,omitempty" yaml:"userEmailDomain,omitempty"`
	AuthSourceID    int64    `json:"authSourceId,omitempty" yaml:"authSourceId,omitempty"`
	CreatorActions  []string `json:"creatorActions" yaml:"creatorActions"`
	ShareActions    []string `json:"shareActions,omitempty" yaml:"shareActions,omitempty"`
}

type ManagedRepositoryRequest struct {
	CatalogID        string
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
	Catalog           string          `json:"catalog"`
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
	if req.CatalogID == "" {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "managed create requires an explicit Catalog")
	}
	cat, ok := ws.Catalogs[req.CatalogID]
	if !ok {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "unknown Catalog %s", req.CatalogID)
	}
	id, err := NormalizeRepositoryID(req.RepositoryID)
	if err != nil || id != req.RepositoryID || strings.TrimSpace(req.CommandID) == "" || strings.TrimSpace(req.Principal) == "" || req.RepositoryID == string(knowledge.SystemRepositoryID) {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "managed create requires a valid repository, commandId, and principal")
	}
	for _, binding := range ws.Deployment.Catalogs {
		if binding.ID == req.RepositoryID {
			return result, kernel.Fail(kernel.ErrTargetRepositoryDenied, "Catalog identity cannot be provisioned as a Snapshot")
		}
	}
	value, _ := managedOperationLocks.LoadOrStore(filepath.Clean(ws.Dir), &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := ValidateDeploymentState(*ws.Deployment); err != nil {
		return result, err
	}
	if err := ws.Registries[req.CatalogID].CheckAuthority(); err != nil {
		return result, err
	}
	req, err = ws.managedRequestForOwner(req)
	if err != nil {
		return result, err
	}
	record, err := reserveManaged(*ws.Deployment, req, func() error {
		if grant == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "managed create requires the configured creator-policy callback")
		}
		if cat.Archived() {
			return kernel.Fail(kernel.ErrCatalogArchived, "Catalog %s is archived", req.CatalogID)
		}
		if _, exists := ws.Store.Get(kernel.RepositoryID(req.RepositoryID)); exists {
			return kernel.Fail(kernel.ErrPreconditionFailed, "repository identity already has a source binding")
		}
		for _, existing := range ws.Catalogs {
			if existing.HasRepository(kernel.RepositoryID(req.RepositoryID)) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "repository identity is already admitted")
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result, err = ws.managedOwnerResult(record)
	if err != nil {
		return result, err
	}
	result.Status = "APPLIED"
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil {
		return result, err
	}
	var source snapshot.Store
	if record.Phase == managedReserved {
		if cat.Archived() {
			return result, kernel.Fail(kernel.ErrCatalogArchived, "Catalog %s is archived", req.CatalogID)
		}
		if record.AccountAllocationID != "" && driver.managedEnsureOwner != nil {
			record.AccountBackendID, err = driver.managedEnsureOwner(record)
			if err != nil {
				return result, err
			}
			if err = saveManaged(ws.Dir, record); err != nil {
				return result, err
			}
		}
		pool, _, err := selectManagedPool(*ws.Deployment, record.Store)
		if err != nil {
			return result, err
		}
		source, record.BackendID, err = driver.managedCreate(pool, record.Binding, record.AllocationID, record.AccountAllocationID != "")
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
	if err := ws.Registries[req.CatalogID].CheckAuthority(); err != nil {
		return result, err
	}
	if cat.Archived() {
		return result, kernel.Fail(kernel.ErrPreconditionFailed, "managed source is no longer in an active Catalog")
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
	ws.inventory.bindings[record.Binding.ID] = record.Binding.homeRepo()
	return nil
}

func validateManagedBindings(c DeploymentConfig, records []managedRecord) error {
	bindings := c
	bindings.ManagedRepositories = nil
	bindings.ManagedStores = nil
	bindings.Repositories = append([]RepositoryBinding(nil), c.Repositories...)
	for _, record := range records {
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
	allowed := []string{"writer.preview", "writer.commit", "writer.receipt.read", "repository.metadata.read", "repository.shares.manage", "catalog.repositories.manage", "governance.proposal.create", "governance.merge", "governance.preview.create", "governance.validate", "governance.validation.record", "projection.read", "dataset.resolve", "file.read", "knowledge.read", "knowledge.provenance", "knowledge.history.read", "knowledge.schema.read", "knowledge.search", "knowledge.relations"}
	if len(actions) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed repositories require an explicit nonempty creatorActions policy")
	}
	seen := map[string]bool{}
	for _, action := range actions {
		if !slices.Contains(allowed, action) || seen[action] {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid or duplicate managed creator action %q", action)
		}
		seen[action] = true
	}
	return nil
}

func validateManagedConfig(c DeploymentConfig) error {
	if c.ManagedRepositories != nil && len(c.ManagedStores) != 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "configure managedRepositories or managedStores, not both")
	}
	for name, pool := range c.ManagedStores {
		if !validManagedUsername(name) {
			return kernel.Fail(kernel.ErrUsageInvalid, "managed store name must be a stable nonempty key")
		}
		if err := validateManagedPool(pool, c); err != nil {
			return err
		}
	}
	pool := c.ManagedRepositories
	if pool == nil {
		return nil
	}
	return validateManagedPool(*pool, c)
}

func validateManagedPool(pool ManagedRepositoryConfig, c DeploymentConfig) error {
	if err := validateCreatorActions(pool.CreatorActions); err != nil {
		return err
	}
	if err := validateShareActions(pool.ShareActions); err != nil {
		return err
	}
	driver, err := authorityFor(pool.Driver)
	if err != nil {
		return err
	}
	if pool.Driver == "" || driver.managedValidate == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed repositories require an explicit supported driver")
	}
	if pool.PublicURL != "" && !validManagedPublicURL(pool.PublicURL) {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed publicURL must be an http(s) URL without credentials, query or fragment")
	}
	return driver.managedValidate(pool, c)
}

func validateShareActions(actions []string) error {
	allowed := []string{"knowledge.read", "knowledge.search", "knowledge.schema.read", "knowledge.provenance", "knowledge.history.read", "knowledge.relations", "dataset.resolve", "file.read"}
	seen := map[string]bool{}
	for _, action := range actions {
		if !slices.Contains(allowed, action) || seen[action] {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid or duplicate managed share action %q", action)
		}
		seen[action] = true
	}
	return nil
}
