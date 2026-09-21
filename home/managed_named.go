package home

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"kc/identity"
	"kc/kernel"
)

func (ws *Home) CreateNamedManagedRepository(req ManagedRepositoryRequest, grant func(ManagedRepositoryGrant) error) (ManagedRepositoryResult, error) {
	if ws == nil || ws.Deployment == nil {
		return ManagedRepositoryResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "managed provisioning requires a deployment")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 128 || strings.IndexFunc(req.Name, unicode.IsControl) >= 0 || !validManagedUsername(req.Principal) || req.RepositoryID != "" || req.CommandID != "" {
		return ManagedRepositoryResult{}, kernel.Fail(kernel.ErrUsageInvalid, "named managed create requires a valid user and name; repository and command are generated")
	}
	// Recover original names before choosing current pools or generating new
	// coordinates. Explicit legacy ownership migration preserves old IDs/URLs.
	{
		records, err := loadManagedRecords(ws.Dir)
		if err != nil {
			return ManagedRepositoryResult{}, err
		}
		var previous *managedRecord
		for _, record := range records {
			owner, err := ws.managedPrincipal(record.Request.Principal)
			if err != nil {
				return ManagedRepositoryResult{}, err
			}
			if owner == req.Principal && record.Request.CatalogID == req.CatalogID && record.Request.Name == req.Name && (req.Store == "" || req.Store == record.Store) {
				if previous != nil {
					return ManagedRepositoryResult{}, kernel.Fail(kernel.ErrUsageInvalid, "multiple allocations contain this name; use the original repository and command")
				}
				copy := record
				previous = &copy
			}
		}
		if previous != nil {
			req.RepositoryID, req.CommandID, req.Store = previous.Request.RepositoryID, previous.Request.CommandID, previous.Request.Store
			return ws.CreateManagedRepository(req, grant)
		}
	}
	if req.Store == "" {
		_, store, err := selectManagedPool(*ws.Deployment, "")
		if err != nil {
			return ManagedRepositoryResult{}, err
		}
		req.Store = store
	}
	pool, store, err := selectManagedPool(*ws.Deployment, req.Store)
	if err != nil {
		return ManagedRepositoryResult{}, err
	}
	req.Store = store
	digest := sha256.Sum256([]byte(kernel.CanonicalDigest(struct{ Catalog, Principal, Name, Store string }{req.CatalogID, req.Principal, req.Name, req.Store})))
	identity := hex.EncodeToString(digest[:])
	req.RepositoryID, err = namedManagedRepositoryID(req, pool, identity)
	if err != nil {
		return ManagedRepositoryResult{}, err
	}
	req.CommandID = "create-" + identity
	return ws.CreateManagedRepository(req, grant)
}

func namedManagedRepositoryID(req ManagedRepositoryRequest, pool ManagedRepositoryConfig, identity string) (string, error) {
	if pool.Driver == "lakefs" {
		return managedLakeFSName(req.Name, req.Principal, identity)
	}
	if strings.TrimSpace(req.Principal) == "" || strings.TrimSpace(identity) == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "named managed create requires a principal")
	}
	return "kr://" + req.Principal + "/repo-" + identity, nil
}

func (ws *Home) ListManagedRepositories(principal string) ([]ManagedRepositoryResult, error) {
	if ws == nil || ws.Deployment == nil || strings.TrimSpace(principal) == "" {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "managed inventory requires an authenticated principal")
	}
	records, err := loadManagedRecords(ws.Dir)
	if err != nil {
		return nil, err
	}
	result := []ManagedRepositoryResult{}
	for _, record := range records {
		owner, err := ws.managedPrincipal(record.Request.Principal)
		if err != nil {
			return nil, err
		}
		if owner == principal {
			item, err := ws.managedOwnerResult(record)
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RepositoryID < result[j].RepositoryID })
	return result, nil
}

func (ws *Home) GetOwnedManagedRepository(principal, id string) (ManagedRepositoryResult, error) {
	rows, err := ws.ListManagedRepositories(principal)
	if err != nil {
		return ManagedRepositoryResult{}, err
	}
	for _, row := range rows {
		if row.RepositoryID == id {
			return row, nil
		}
	}
	return ManagedRepositoryResult{}, kernel.Fail(kernel.ErrTargetRepositoryDenied, "managed repository is not owned by this principal")
}

// ManagedRepositoryShareActions returns the frozen deployment sharing policy.
// The application evaluates current identity, delegation authority and grants.
func (ws *Home) ManagedRepositoryShareActions(id string) ([]string, error) {
	records, err := loadManagedRecords(ws.Dir)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Request.RepositoryID == id {
			return append([]string(nil), record.ShareActions...), nil
		}
	}
	return nil, kernel.Fail(kernel.ErrTargetRepositoryDenied, "repository is not a managed allocation")
}

func managedResult(record managedRecord) ManagedRepositoryResult {
	owner, store := record.Owner, record.Store
	if owner == "" {
		owner = record.Request.Principal
	}
	if store == "" {
		store = record.Binding.Driver
	}
	return ManagedRepositoryResult{Catalog: record.Request.CatalogID, RepositoryID: record.Request.RepositoryID, CommandID: record.Request.CommandID, Status: "REPLAYED", Head: record.Head, Name: record.Name, Owner: owner, Store: store, ManagementURL: record.ManagementURL, ManagementState: record.ManagementState, ProviderURL: record.ProviderURL, ProvisioningState: record.Phase}
}

func selectManagedPool(c DeploymentConfig, name string) (ManagedRepositoryConfig, string, error) {
	if c.ManagedRepositories != nil && len(c.ManagedStores) == 0 {
		if name != "" && name != c.ManagedRepositories.Driver {
			return ManagedRepositoryConfig{}, "", kernel.Fail(kernel.ErrUsageInvalid, "unknown managed store %q", name)
		}
		return *c.ManagedRepositories, c.ManagedRepositories.Driver, nil
	}
	if len(c.ManagedStores) == 0 {
		return ManagedRepositoryConfig{}, "", kernel.Fail(kernel.ErrPreconditionFailed, "managed repository provisioning is not configured")
	}
	if name == "" && len(c.ManagedStores) == 1 {
		for key := range c.ManagedStores {
			name = key
		}
	}
	if name == "" {
		return ManagedRepositoryConfig{}, "", kernel.Fail(kernel.ErrUsageInvalid, "multiple managed stores are available; select a store")
	}
	pool, ok := c.ManagedStores[name]
	if !ok {
		return ManagedRepositoryConfig{}, "", kernel.Fail(kernel.ErrUsageInvalid, "unknown managed store %q", name)
	}
	return pool, name, nil
}

func validManagedUsername(value string) bool {
	_, err := identity.CanonicalUsername(value)
	return err == nil
}

func validManagedPublicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
