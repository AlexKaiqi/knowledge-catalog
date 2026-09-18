package home

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// ConnectionPolicy explicitly delegates origins and initial repository-scoped
// actions. Authentication or connection ownership does not imply these grants.
type ConnectionPolicy struct {
	AllowedOrigins []string `json:"allowedOrigins" yaml:"allowedOrigins"`
	CreatorActions []string `json:"creatorActions" yaml:"creatorActions"`
	ShareActions   []string `json:"shareActions,omitempty" yaml:"shareActions,omitempty"`
}

type ConnectionRequest struct {
	Catalog    string
	Repository string
	Driver     string
	URL        string
	Principal  string
	Credential string `json:"-"`
}

type ConnectionResult struct {
	Catalog       string          `json:"catalog"`
	RepositoryID  string          `json:"repositoryId"`
	Driver        string          `json:"driver"`
	Owner         string          `json:"owner"`
	ManagementURL string          `json:"managementURL"`
	Status        string          `json:"status"`
	Revision      uint64          `json:"revision"`
	Head          kernel.CommitID `json:"head"`
}

func validConnectionURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(raw, "\\\r\n") || (u.Path != "" && path.Clean(u.Path) != strings.TrimRight(u.Path, "/")) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "connection URL must be a canonical http(s) URL without credentials, query, or fragment")
	}
	return u, nil
}

func connectionOrigin(raw string) (string, error) {
	u, err := validConnectionURL(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "connection URL requires an existing owner/repository")
	}
	u.Path = strings.Join(parts[:len(parts)-2], "/")
	if u.Path != "" {
		u.Path = "/" + u.Path
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func validateConnectionActions(actions []string) error {
	if !slices.Contains(actions, "repository.connections.manage") {
		return kernel.Fail(kernel.ErrUsageInvalid, "connection creatorActions must explicitly include repository.connections.manage")
	}
	rest := make([]string, 0, len(actions))
	seen := false
	for _, action := range actions {
		if action == "repository.connections.manage" {
			if seen {
				return kernel.Fail(kernel.ErrUsageInvalid, "duplicate connection creator action")
			}
			seen = true
		} else {
			rest = append(rest, action)
		}
	}
	if len(rest) > 0 {
		return validateCreatorActions(rest)
	}
	return nil
}

func validateConnectionPolicy(policy *ConnectionPolicy) error {
	if policy == nil {
		return nil
	}
	if len(policy.AllowedOrigins) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "connections requires explicit allowedOrigins")
	}
	seen := map[string]bool{}
	for _, origin := range policy.AllowedOrigins {
		u, err := validConnectionURL(origin)
		if err != nil {
			return err
		}
		canonical := strings.TrimRight(u.String(), "/")
		if canonical != origin || seen[origin] {
			return kernel.Fail(kernel.ErrUsageInvalid, "connection origins must be unique canonical provider origins")
		}
		seen[origin] = true
	}
	if err := validateConnectionActions(policy.CreatorActions); err != nil {
		return err
	}
	return validateShareActions(policy.ShareActions)
}

func validateConnectionCredential(credential string) error {
	if strings.TrimSpace(credential) == "" || len(credential) > 65536 || strings.ContainsAny(credential, "\r\n") {
		return kernel.Fail(kernel.ErrUsageInvalid, "connection requires a nonempty credential without line breaks")
	}
	return nil
}

func connectionResult(r connectionRecord) ConnectionResult {
	return ConnectionResult{Catalog: r.Catalog, RepositoryID: r.Binding.ID, Driver: r.Binding.Driver, Owner: r.Owner, ManagementURL: r.Binding.DSN, Status: r.Phase, Revision: r.Revision, Head: r.InitialHead}
}

// Connection errors deliberately exclude provider bodies and private inputs.
func connectionFailure(err error) error {
	if err == nil {
		return nil
	}
	code := kernel.CodeOf(err)
	if code == "" {
		code = kernel.ErrPreconditionFailed
	}
	return kernel.Fail(code, "connected authority could not be verified; check its availability, identity, and credential")
}

func openConnectionRecord(r connectionRecord, credential string) (snapshot.Store, error) {
	driver, err := authorityFor(r.Binding.Driver)
	if err != nil || driver.connectionOpen == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "connection driver is unavailable")
	}
	source, _, err := driver.connectionOpen(r.Binding, credential, r.BackendID)
	if err != nil {
		return nil, connectionFailure(err)
	}
	if !source.HasCommit(r.InitialHead) {
		closeManagedSource(source)
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "connected initial published commit is unavailable")
	}
	return source, nil
}

func (ws *Home) connectionLock() *sync.Mutex {
	value, _ := managedOperationLocks.LoadOrStore(filepath.Clean(ws.Dir), &sync.Mutex{})
	return value.(*sync.Mutex)
}

// ConnectRepository performs only read-only validation on the external source.
// Its durable receipt freezes authority and policy; a replay never regrants.
func (ws *Home) ConnectRepository(req ConnectionRequest, grant func(RepositoryInitialGrant) error) (ConnectionResult, error) {
	var result ConnectionResult
	if ws == nil || ws.Deployment == nil || ws.readOnly {
		return result, kernel.Fail(kernel.ErrPreconditionFailed, "connections require a writable deployment")
	}
	if req.Driver != "gitea" {
		return result, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "self-service connections currently support existing Gitea repositories")
	}
	id, err := NormalizeCatalogID(req.Repository)
	if err != nil || id != req.Repository || req.Repository == string(knowledge.SystemRepositoryID) || req.Principal == "" {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "connection requires a valid repository and principal")
	}
	if err := validateConnectionCredential(req.Credential); err != nil {
		return result, err
	}
	origin, err := connectionOrigin(req.URL)
	if err != nil {
		return result, err
	}
	policy := ws.Deployment.Connections
	if policy == nil || !slices.Contains(policy.AllowedOrigins, origin) {
		return result, kernel.Fail(kernel.ErrForbidden, "connection provider is not approved by deployment policy")
	}
	lock := ws.connectionLock()
	lock.Lock()
	defer lock.Unlock()
	if err := ValidateDeploymentState(*ws.Deployment); err != nil {
		return result, err
	}
	cat, _, err := ws.UseCatalog(req.Catalog)
	if err != nil {
		return result, err
	}
	if err := ws.Registries[req.Catalog].CheckAuthority(); err != nil {
		return result, err
	}
	record, credential, readErr := readConnection(ws.Dir, req.Repository)
	if readErr == nil {
		if record.Owner != req.Principal {
			return result, kernel.Fail(kernel.ErrForbidden, "connection belongs to another principal")
		}
		if record.Catalog != req.Catalog || record.Binding.DSN != req.URL || record.Binding.Driver != req.Driver {
			return result, kernel.Fail(kernel.ErrPreconditionFailed, "existing connection has a fixed authority; use its original coordinates")
		}
		if credential != req.Credential {
			return result, kernel.Fail(kernel.ErrPreconditionFailed, "use credential rotation for an existing connection")
		}
		if record.Phase == "READY" {
			return connectionResult(record), nil
		}
	} else if kernel.CodeOf(readErr) != kernel.ErrTargetRepositoryDenied {
		return result, readErr
	} else {
		if grant == nil {
			return result, kernel.Fail(kernel.ErrPreconditionFailed, "connection initial policy callback is required")
		}
		if cat.Archived() {
			return result, kernel.Fail(kernel.ErrCatalogArchived, "Catalog is archived")
		}
		if _, exists := ws.Store.Get(kernel.RepositoryID(req.Repository)); exists {
			return result, kernel.Fail(kernel.ErrPreconditionFailed, "repository already has an authority binding")
		}
		for catalogID, existing := range ws.Catalogs {
			if catalogID == req.Repository || existing.HasRepository(kernel.RepositoryID(req.Repository)) {
				return result, kernel.Fail(kernel.ErrPreconditionFailed, "repository identity is already in use")
			}
		}
		binding := RepositoryBinding{ID: req.Repository, Driver: req.Driver, DSN: req.URL}
		driver, _ := authorityFor(req.Driver)
		source, backend, err := driver.connectionOpen(binding, req.Credential, "")
		if err != nil {
			return result, connectionFailure(err)
		}
		head, err := source.Head(snapshot.DefaultRef)
		closeManagedSource(source)
		if err != nil {
			return result, connectionFailure(err)
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return result, err
		}
		record = connectionRecord{AllocationID: hex.EncodeToString(nonce[:]), Owner: req.Principal, Catalog: req.Catalog, Binding: binding, BackendID: backend, InitialHead: head, Actions: append([]string(nil), policy.CreatorActions...), ShareActions: append([]string(nil), policy.ShareActions...), Phase: "CONNECTED"}
		credential = req.Credential
		if err := saveConnection(ws.Dir, &record, credential, 0); err != nil {
			return result, err
		}
	}
	if _, exists := ws.Store.Get(kernel.RepositoryID(req.Repository)); !exists {
		if err := ws.Store.Add(&connectedSource{dir: ws.Dir, id: kernel.RepositoryID(req.Repository)}); err != nil {
			return result, err
		}
	}
	ws.inventory.mu.Lock()
	ws.inventory.bindings[record.Binding.ID] = record.Binding.homeRepo()
	ws.inventory.mu.Unlock()
	if cat.Archived() {
		return result, kernel.Fail(kernel.ErrCatalogArchived, "Catalog is archived")
	}
	if grant == nil {
		return result, kernel.Fail(kernel.ErrPreconditionFailed, "connection initial policy callback is required")
	}
	if err := grant(RepositoryInitialGrant{AllocationID: record.AllocationID, Principal: record.Owner, RepositoryID: record.Binding.ID, Actions: append([]string(nil), record.Actions...)}); err != nil {
		return result, err
	}
	record.Phase = "READY"
	if err := saveConnection(ws.Dir, &record, credential, record.Revision); err != nil {
		return result, err
	}
	return connectionResult(record), nil
}

func (ws *Home) ownedConnection(principal, id string) (connectionRecord, string, error) {
	if ws == nil || ws.Deployment == nil {
		return connectionRecord{}, "", kernel.Fail(kernel.ErrPreconditionFailed, "connections require a deployment")
	}
	record, credential, err := readConnection(ws.Dir, id)
	if err != nil {
		return record, "", err
	}
	if principal == "" || record.Owner != principal {
		return record, "", kernel.Fail(kernel.ErrForbidden, "connection belongs to another principal")
	}
	return record, credential, nil
}

func (ws *Home) GetConnection(principal, id string) (ConnectionResult, error) {
	r, _, err := ws.ownedConnection(principal, id)
	if err != nil {
		return ConnectionResult{}, err
	}
	return connectionResult(r), nil
}
func (ws *Home) CheckConnection(principal, id string) (ConnectionResult, error) {
	r, credential, err := ws.ownedConnection(principal, id)
	if err != nil {
		return ConnectionResult{}, err
	}
	source, err := openConnectionRecord(r, credential)
	if err != nil {
		return ConnectionResult{}, err
	}
	defer closeManagedSource(source)
	result := connectionResult(r)
	result.Head, err = source.Head(snapshot.DefaultRef)
	return result, connectionFailure(err)
}
func (ws *Home) RotateConnectionCredential(principal, id, credential string) (ConnectionResult, error) {
	if ws == nil || ws.readOnly {
		return ConnectionResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "rotation requires a writable deployment")
	}
	if err := validateConnectionCredential(credential); err != nil {
		return ConnectionResult{}, err
	}
	lock := ws.connectionLock()
	lock.Lock()
	defer lock.Unlock()
	r, _, err := ws.ownedConnection(principal, id)
	if err != nil {
		return ConnectionResult{}, err
	}
	source, err := openConnectionRecord(r, credential)
	if err != nil {
		return ConnectionResult{}, err
	}
	closeManagedSource(source)
	if err := saveConnection(ws.Dir, &r, credential, r.Revision); err != nil {
		return ConnectionResult{}, err
	}
	return connectionResult(r), nil
}

func (ws *Home) RepositoryShareActions(id string) ([]string, error) {
	actions, err := ws.ManagedRepositoryShareActions(id)
	if err == nil || kernel.CodeOf(err) != kernel.ErrTargetRepositoryDenied {
		return actions, err
	}
	r, _, err := readConnection(ws.Dir, id)
	if err != nil {
		return nil, err
	}
	if r.Phase != "READY" {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "connection is not ready")
	}
	return append([]string(nil), r.ShareActions...), nil
}
