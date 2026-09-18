package home

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/hook"
	"kc/internal/jsonfile"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/observability"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

const deploymentMarker = "deployment-state.json"

// Initialization receipts distinguish a new configured Catalog from a lost
// authority ref. They are recovery evidence, never membership or source config.
type deploymentState struct {
	Version                    int      `json:"version"`
	InitializedCatalogs        []string `json:"initializedCatalogs"`
	ManagedStoreInitialized    bool     `json:"managedStoreInitialized,omitempty"`
	IdentityStoreInitialized   bool     `json:"identityStoreInitialized,omitempty"`
	ConnectionStoreInitialized bool     `json:"connectionStoreInitialized,omitempty"`
}

func (s deploymentState) initialized(id string) bool {
	for _, known := range s.InitializedCatalogs {
		if known == id {
			return true
		}
	}
	return false
}
func readDeploymentState(c DeploymentConfig) (deploymentState, error) {
	var state deploymentState
	err := jsonfile.Read(filepath.Join(c.StateDir, deploymentMarker), &state)
	return state, err
}

var durableFiles = []string{"allow.json", "hooks.json", "gates.json", "writer.db", "control.json", "system.jsonl", "access.jsonl", "feedback.jsonl", "retrieval.jsonl", "refine.jsonl", "audit.jsonl"}

func writeEvidencePolicy(dir string, cfg *EvidenceConfig) error {
	policy := observability.DefaultFileStorePolicy()
	if cfg != nil {
		if cfg.HotRetention != "" {
			policy.HotRetention = cfg.HotRetention
		}
		if cfg.MaxBytes > 0 {
			policy.MaxBytes = cfg.MaxBytes
		}
	}
	return observability.WriteFileStorePolicy(dir, policy)
}

// ValidateDeploymentState fails closed when a durable volume is absent or
// incomplete. Missing policy is never interpreted as an empty policy.
func ValidateDeploymentState(c DeploymentConfig) error {
	return validateDeploymentState(c, false)
}

func validateDeploymentState(c DeploymentConfig, allowIdentityUpgrade bool) error {
	marker, err := readDeploymentState(c)
	if err != nil || marker.Version != 1 || len(marker.InitializedCatalogs) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "deployment state is unavailable; initialize explicitly or restore the durable volume")
	}
	for _, name := range durableFiles {
		info, err := os.Stat(filepath.Join(c.StateDir, name))
		if err != nil || !info.Mode().IsRegular() {
			return kernel.Fail(kernel.ErrPreconditionFailed, "durable deployment state %s is unavailable; restore the volume", name)
		}
	}
	if marker.IdentityStoreInitialized {
		if err := validateIdentityBindings(c.StateDir); err != nil {
			return err
		}
	} else if !allowIdentityUpgrade {
		return kernel.Fail(kernel.ErrPreconditionFailed, "identity bindings require explicit deployment init; existing grants are not migrated by login")
	}
	for _, name := range []string{"allow.json", "hooks.json", "gates.json", "control.json"} {
		raw, err := os.ReadFile(filepath.Join(c.StateDir, name))
		if err != nil {
			return err
		}
		if !json.Valid(raw) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "durable deployment state %s is invalid", name)
		}
	}
	if _, err := gate.Read(c.StateDir); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable gate policy is invalid")
	}
	if _, err := hook.Read(c.StateDir); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable hook policy is invalid")
	}
	if _, err := controlplane.NewFileControlState(filepath.Join(c.StateDir, "control.json")).LoadBundle(); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable control state is invalid")
	}
	if _, err := commandlog.OpenBoltStore(filepath.Join(c.StateDir, "writer.db")); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable Writer ledger is invalid")
	}
	if marker.ManagedStoreInitialized {
		records, err := loadManagedRecords(c.StateDir)
		if err != nil {
			return err
		}
		if err := validateManagedBindings(c, records); err != nil {
			return err
		}
	} else {
		if _, err := os.Stat(filepath.Join(c.StateDir, managedLedgerFile)); err == nil {
			records, readErr := loadManagedRecords(c.StateDir)
			if readErr != nil || len(records) > 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "managed allocation initialization receipt is unavailable; restore durable state")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if c.ManagedRepositories != nil || len(c.ManagedStores) != 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "managed repositories require explicit deployment init to initialize their durable ledger")
		}
	}
	return validateConnectionState(c, marker, allowIdentityUpgrade)
}

// InitializeDeployment is an explicit operator operation. The Snapshot
// container already exists or is created here; only Catalog registry files
// may be initialized. seedPolicies owns the application's grant contract and runs once, in staging.
func InitializeDeployment(c DeploymentConfig, seedPolicies func(string, string) error) error {
	if err := c.Validate(); err != nil {
		return err
	}
	_, markerErr := os.Stat(filepath.Join(c.StateDir, deploymentMarker))
	initialized := markerErr == nil
	state := deploymentState{Version: 1}
	if initialized {
		var err error
		state, err = readDeploymentState(c)
		if err != nil {
			return err
		}
	}
	if initialized {
		validation := c
		if !state.ManagedStoreInitialized {
			validation.ManagedRepositories = nil
			validation.ManagedStores = nil
		}
		if err := validateDeploymentState(validation, true); err != nil {
			return err
		}
	} else {
		if !errors.Is(markerErr, os.ErrNotExist) {
			return markerErr
		}
		if entries, err := os.ReadDir(c.StateDir); err == nil && len(entries) > 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "stateDir contains unrecognized durable data; restore its deployment marker")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if c.BootstrapPrincipal == "" || seedPolicies == nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "initial deployment requires bootstrapPrincipal")
		}
	}
	if !initialized {
		for _, b := range c.Catalogs {
			reg, err := openCatalogRegistry(b)
			if err == nil {
				_ = reg.Close()
				return kernel.Fail(kernel.ErrPreconditionFailed, "Catalog authority already exists but durable deployment state is missing; restore the state volume")
			}
			if kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
				return err
			}
		}
	}
	store := snapshot.NewRegistry()
	defer store.Close()
	if err := store.Add(knowledge.NewSystemRepository()); err != nil {
		return err
	}
	for _, b := range c.Catalogs {
		var reg *catalog.Registry
		var err error
		if initialized {
			reg, err = openCatalogRegistry(b)
		}
		if kernel.CodeOf(err) == kernel.ErrVersionUnresolved && state.initialized(b.ID) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "Catalog %s was initialized but its authority ref is missing; restore the Catalog Snapshot authority", b.ID)
		}
		if !initialized || kernel.CodeOf(err) == kernel.ErrVersionUnresolved {
			if reg != nil {
				_ = reg.Close()
			}
			reg, err = createCatalogRegistry(b)
		}
		if err != nil {
			return err
		}
		cat, err := catalog.NewCatalog(store, reg)
		if err != nil {
			_ = reg.Close()
			return err
		}
		if !cat.Archived() {
			if err := cat.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
				_ = reg.Close()
				return err
			}
		}
		_ = reg.Close()
		if !state.initialized(b.ID) {
			state.InitializedCatalogs = append(state.InitializedCatalogs, b.ID)
		}
	}
	if initialized {
		if !state.ConnectionStoreInitialized {
			if err := createConnectionLedger(filepath.Join(c.StateDir, connectionLedgerFile)); err != nil {
				return err
			}
			state.ConnectionStoreInitialized = true
		}
		if !state.IdentityStoreInitialized {
			if err := initializeIdentityBindings(c.StateDir); err != nil {
				return err
			}
			state.IdentityStoreInitialized = true
		}
		if !state.ManagedStoreInitialized {
			if err := createManagedLedger(filepath.Join(c.StateDir, managedLedgerFile)); err != nil {
				return err
			}
			state.ManagedStoreInitialized = true
		}
		return jsonfile.Write(filepath.Join(c.StateDir, deploymentMarker), state)
	}
	if err := os.MkdirAll(filepath.Dir(c.StateDir), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(c.StateDir), ".kc-state-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := seedPolicies(staging, c.BootstrapPrincipal); err != nil {
		return err
	}
	if err := WriteRepositoryAccess(staging, RepositoryAccessFile{Repositories: append([]RepositoryAccess{}, c.RepositoryAccess...)}); err != nil {
		return err
	}
	if err := initializeIdentityBindings(staging); err != nil {
		return err
	}
	state.IdentityStoreInitialized = true
	if err := hook.Write(staging, hook.File{}); err != nil {
		return err
	}
	if err := gate.Write(staging, gate.File{}); err != nil {
		return err
	}
	if err := controlplane.NewFileControlState(filepath.Join(staging, "control.json")).SaveBundle(map[string]controlplane.ControlState{}); err != nil {
		return err
	}
	if _, err := commandlog.New(commandlog.NewBoltStore(filepath.Join(staging, "writer.db"))); err != nil {
		return err
	}
	if err := createManagedLedger(filepath.Join(staging, managedLedgerFile)); err != nil {
		return err
	}
	state.ManagedStoreInitialized = true
	if err := createConnectionLedger(filepath.Join(staging, connectionLedgerFile)); err != nil {
		return err
	}
	state.ConnectionStoreInitialized = true
	for _, name := range durableFiles[5:] {
		if err := os.WriteFile(filepath.Join(staging, name), nil, 0600); err != nil {
			return err
		}
	}
	if err := writeEvidencePolicy(staging, c.Evidence); err != nil {
		return err
	}
	if err := jsonfile.Write(filepath.Join(staging, deploymentMarker), state); err != nil {
		return err
	}
	// Rename installs a complete state bundle. An existing nonempty destination
	// is never replaced; concurrent initialization has one winner.
	if err := os.Rename(staging, c.StateDir); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(c.StateDir))
	if err != nil {
		return err
	}
	syncErr := parent.Sync()
	closeErr := parent.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return ValidateDeploymentState(c)
}

func OpenDeployment(c DeploymentConfig) (*Home, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateDeploymentState(c); err != nil {
		return nil, err
	}
	state, err := readDeploymentState(c)
	if err != nil {
		return nil, err
	}
	for _, binding := range c.Catalogs {
		if !state.initialized(binding.ID) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Catalog %s requires explicit deployment initialization", binding.ID)
		}
	}
	store := snapshot.NewRegistry()
	registries := map[string]*catalog.Registry{}
	fail := func(err error) (*Home, error) {
		for _, reg := range registries {
			_ = reg.Close()
		}
		_ = store.Close()
		return nil, err
	}
	systemBound := false
	file := HomeFile{}
	for _, b := range c.Repositories {
		item := b.homeRepo()
		file.Repos = append(file.Repos, item)
		source, err := openExistingAuthority(item)
		if err != nil {
			return fail(err)
		}
		if err := store.Add(source); err != nil {
			return fail(err)
		}
		if b.ID == string(knowledge.SystemRepositoryID) {
			systemBound = true
		}
	}
	if state.ManagedStoreInitialized {
		records, err := loadManagedRecords(c.StateDir)
		if err != nil {
			return fail(err)
		}
		for _, record := range records {
			if record.Phase == managedReserved {
				continue
			}
			source, err := restoreManagedSource(record)
			if err != nil {
				return fail(err)
			}
			if err := store.Add(source); err != nil {
				closeManagedSource(source)
				return fail(err)
			}
			file.Repos = append(file.Repos, record.Binding.homeRepo())
		}
	}
	// Restore external handles without requiring working credentials.
	if state.ConnectionStoreInitialized {
		records, err := loadConnectionRecords(c.StateDir)
		if err != nil {
			return fail(err)
		}
		for _, record := range records {
			if err := store.Add(&connectedSource{dir: c.StateDir, id: kernel.RepositoryID(record.Binding.ID)}); err != nil {
				return fail(err)
			}
			file.Repos = append(file.Repos, record.Binding.homeRepo())
		}
	}
	if !systemBound {
		if err := store.Add(knowledge.NewSystemRepository()); err != nil {
			return fail(err)
		}
	}
	rd := reader.NewReader(store)
	system, err := rd.Require(knowledge.SystemRepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return fail(err)
	}
	systemSource, _ := store.Get(knowledge.SystemRepositoryID)
	systemHead, err := systemSource.Head(snapshot.DefaultRef)
	if err != nil {
		return fail(err)
	}
	for _, op := range knowledge.SystemSchemaOperations() {
		current, err := system.Read(op.Address.ObjectID, systemHead)
		if err != nil {
			return fail(err)
		}
		if kernel.CanonicalDigest(current.Value) != kernel.CanonicalDigest(op.Value) {
			return fail(kernel.Fail(kernel.ErrPreconditionFailed, "System publication differs from the binary trust root"))
		}
	}
	catalogs := map[string]*catalog.Catalog{}
	for _, b := range c.Catalogs {
		reg, err := openCatalogRegistry(b)
		if err != nil {
			return fail(err)
		}
		cat, err := catalog.NewCatalog(store, reg)
		if err != nil {
			_ = reg.Close()
			return fail(err)
		}
		catalogs[b.ID], registries[b.ID] = cat, reg
		file.Catalogs = append(file.Catalogs, HomeCatalog{ID: b.ID, Dir: c.CatalogCache(b)})
	}
	stores := c.runtimeStores()
	if err := stores.ValidateProfile(); err != nil {
		return fail(err)
	}
	if !fileExists(repositoryAccessPath(c.StateDir)) {
		if err := WriteRepositoryAccess(c.StateDir, RepositoryAccessFile{Repositories: append([]RepositoryAccess{}, c.RepositoryAccess...)}); err != nil {
			return fail(err)
		}
	}
	if err := writeEvidencePolicy(c.StateDir, c.Evidence); err != nil {
		return fail(err)
	}
	ws, err := assemble(c.StateDir, file, stores, store, catalogs, registries, true)
	if err != nil {
		return fail(err)
	}
	ws.Deployment = &c
	return ws, nil
}

// AttachRepository validates an existing configured Snapshot before admitting
// its identity in one Catalog commit. Source contents are never changed.
func (ws *Home) AttachRepository(catalogID string, id kernel.RepositoryID) error {
	cat, _, err := ws.UseCatalog(catalogID)
	if err != nil {
		return err
	}
	if ws.Deployment != nil {
		if _, err := ws.Deployment.Binding(id); err != nil {
			if err := ValidateDeploymentState(*ws.Deployment); err != nil {
				return err
			}
			record, credential, connectionErr := readConnection(ws.Dir, string(id))
			if connectionErr == nil {
				source, connectionErr := openConnectionRecord(record, credential)
				if connectionErr != nil {
					return connectionErr
				}
				closeManagedSource(source)
			} else if kernel.CodeOf(connectionErr) == kernel.ErrTargetRepositoryDenied {
				if err := ws.verifyManagedBinding(id); err != nil {
					return err
				}
			} else {
				return connectionErr
			}
		}
	}
	if _, ok := ws.Store.Get(id); !ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot %s is unavailable", id)
	}
	return cat.RegisterRepository(id)
}

// PublishDeploymentSystem is explicit provisioning through the System Writer.
// It neither registers a business Snapshot nor changes deployment bindings.
func PublishDeploymentSystem(c DeploymentConfig) (map[string]any, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateDeploymentState(c); err != nil {
		return nil, err
	}
	b, err := c.Binding(knowledge.SystemRepositoryID)
	if err != nil {
		return nil, err
	}
	repo, err := openAuthority(b.Dir, b.homeRepo())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closer, ok := repo.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	published, err := writer.PublishSystem(repo)
	if err != nil {
		return nil, err
	}
	status := SystemRepositoryStatus(published.Commit)
	status["seeded"] = published.Seeded
	return status, nil
}
