package home

import (
	"fmt"
	"path/filepath"
	"sync"

	"kc/catalog"
	"kc/controlplane"
	"kc/index"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	readcache "kc/retrieval/cache"
	"kc/snapshot"
	"kc/snapshot/commandlog"
	"kc/snapshot/treewriter"
)

// Assembling the live object graph for one command: mount the members, build the
// Catalogs, then hang Writer / Reader / ControlPlane / index off them. This is
// the only place that knows the wiring order, and it changes when the set of
// collaborators changes.
//
// What lives in this --home is home_discover.go; which engine backs a member
// is home_mount.go. Per-verb behaviour stays in cli/verbs_*.go.

type Home struct {
	inventory    *homeInventory
	readOnly     bool
	closeOnce    sync.Once
	closeErr     error
	controlMu    sync.Mutex
	Deployment   *DeploymentConfig
	Dir          string
	Store        *snapshot.Registry
	Commands     *commandlog.Ledger
	Writer       *writer.Writer
	TreeWriter   *treewriter.Writer
	Reader       *reader.Reader
	Catalogs     map[string]*catalog.Catalog
	Registries   map[string]*catalog.Registry
	Catalog      *catalog.Catalog
	Registry     *catalog.Registry
	ControlPlane *controlplane.ControlPlane
	ControlStore *controlplane.FileControlState
	File         HomeFile
	Control      controlplane.ControlState
	Controls     map[string]controlplane.ControlState
	controlID    string
	Journal      journal.Journal
	Index        *index.Index
	Projection   *index.Controller
	Hydrator     knowledge.Hydrator
	ReadCache    *readcache.Cache
	Stores       StoresFile
}

func Open(home string) (*Home, error) {
	file, err := ReadHome(home)
	if err != nil {
		return nil, err
	}
	if len(file.Catalogs) == 0 {
		return nil, fmt.Errorf("no catalog fixture at %s; production requires kc deployment init --config (fixture %s)", home, home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return nil, err
	}
	if err := stores.ValidateProfile(); err != nil {
		return nil, err
	}
	store, err := openMembers(home, file, stores)
	if err != nil {
		return nil, err
	}
	catalogs, registries, err := OpenCatalogs(home, file, store)
	if err != nil {
		return nil, err
	}
	return assemble(home, file, stores, store, catalogs, registries, false)
}

func assemble(home string, file HomeFile, stores StoresFile, store *snapshot.Registry, catalogs map[string]*catalog.Catalog, registries map[string]*catalog.Registry, recovering bool) (*Home, error) {
	defaultID := file.Catalogs[0].ID
	var ledgerStore commandlog.Store = commandlog.NewBoltStore(filepath.Join(home, "writer.db"))
	if recovering {
		existing, err := commandlog.OpenBoltStore(filepath.Join(home, "writer.db"))
		if err != nil {
			return nil, err
		}
		ledgerStore = existing
	}
	commands, err := commandlog.New(ledgerStore)
	if err != nil {
		return nil, err
	}
	w, err := writer.NewWriter(store, commands)
	if err != nil {
		return nil, err
	}
	tw, err := treewriter.New(store, commands)
	if err != nil {
		return nil, err
	}
	controlStore := controlplane.NewFileControlState(filepath.Join(home, "control.json"))
	bundle, err := loadControlBundle(controlStore, defaultID)
	if err != nil {
		return nil, err
	}
	sys := journal.NewFile(systemJournalPath(home))
	w.SetJournal(sys)
	tw.SetJournal(sys)
	rd := reader.NewReader(store)
	rd.SetJournal(sys)
	for _, cat := range catalogs {
		cat.SetJournal(sys)
	}
	plane := controlplane.New(store, w, catalogs[defaultID])
	plane.SetJournal(sys)
	idxDir, err := ResolveStoreDir(home, stores.Layout.Projections, defaultProjectionsDir)
	if err != nil {
		return nil, err
	}
	ws := &Home{
		inventory:    &homeInventory{bindings: map[string]HomeRepo{}},
		Dir:          home,
		Store:        store,
		Commands:     commands,
		Writer:       w,
		TreeWriter:   tw,
		Reader:       rd,
		Catalogs:     catalogs,
		Registries:   registries,
		Catalog:      catalogs[defaultID],
		Registry:     registries[defaultID],
		ControlPlane: plane,
		ControlStore: controlStore,
		File:         file,
		Control:      bundle[defaultID],
		Controls:     bundle,
		controlID:    defaultID,
		Journal:      sys,
		Index:        index.NewIndexEngine(idxDir, indexOpener(file, stores)),
		Stores:       stores,
	}
	for _, binding := range file.Repos {
		ws.inventory.bindings[binding.ID] = binding
	}
	if err := ws.configureHydration(); err != nil {
		return nil, err
	}
	if stores.Index != "none" || ws.ReadCache != nil {
		projection := ws.Index
		if stores.Index == "none" {
			projection = nil
		}
		controller, err := index.NewController(
			projection,
			index.NewTargetStore(filepath.Join(idxDir, "controller.db")),
			func(id kernel.RepositoryID) (knowledge.Repository, error) {
				return rd.Require(id, kernel.ErrCapabilityUnsatisfied)
			},
		)
		if err != nil {
			return nil, err
		}
		controller.SetInventory(func() ([]kernel.RepositoryID, error) {
			return store.IDs(), nil
		})
		ws.Projection = controller
		if projection != nil {
			var servingCatalogs []*catalog.Catalog
			for _, cat := range catalogs {
				servingCatalogs = append(servingCatalogs, cat)
			}
			if err := controller.RegisterConsumer(datasetProjectionConsumer{catalogs: servingCatalogs, controller: controller}); err != nil {
				return nil, err
			}
		}
		if ws.ReadCache != nil {
			if err := controller.RegisterConsumer(cacheWarmer{cache: ws.ReadCache}); err != nil {
				return nil, err
			}
		}
	}
	ws.wireSidecars()
	return ws, nil
}

// openMembers opens every discovered Repository. One bad member fails the command:
// a partially attached Store Directory would silently answer Workspace reads with fewer sources.
func openMembers(home string, file HomeFile, stores StoresFile) (*snapshot.Registry, error) {
	store := snapshot.NewRegistry()
	var system snapshot.Store
	for _, repo := range file.Repos {
		if repo.DSN != "" {
			if err := snapshot.RejectConfiguredSecret(repo.Driver, repo.DSN, "KC_GITEA_TOKEN"); err != nil {
				return nil, fmt.Errorf("repository %s: %w", repo.ID, err)
			}
		}
		opened, err := openAttachedRepository(home, repo, stores)
		if err != nil {
			return nil, err
		}
		if repo.ID == string(knowledge.SystemRepositoryID) {
			system = opened
			continue
		}
		if err := store.Add(opened); err != nil {
			return nil, err
		}
	}
	if system == nil {
		system = knowledge.NewSystemRepository()
	}
	if err := store.Add(system); err != nil {
		return nil, err
	}
	return store, nil
}

// OpenCatalogs builds every Catalog over the same Store, so a Repository registered in
// two Catalogs is one attached Store, not two.
func OpenCatalogs(home string, file HomeFile, store *snapshot.Registry) (map[string]*catalog.Catalog, map[string]*catalog.Registry, error) {
	catalogs := map[string]*catalog.Catalog{}
	registries := map[string]*catalog.Registry{}
	for _, item := range file.Catalogs {
		registry, err := catalog.NewRegistry(filepath.Join(home, item.Dir), item.ID)
		if err != nil {
			return nil, nil, err
		}
		cat, err := catalog.NewCatalog(store, registry)
		if err != nil {
			return nil, nil, err
		}
		// The built-in protocol publication is a deployment invariant, not a
		// user-attached business Repository. Registering here also migrates an
		// existing Home on its first open after upgrade.
		if !cat.Archived() {
			if err := cat.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
				return nil, nil, err
			}
		}
		catalogs[item.ID] = cat
		registries[item.ID] = registry
	}
	return catalogs, registries, nil
}

// loadControlBundle reads per-Catalog ControlState. The "" key is from before
// control state was keyed by Catalog; it belongs to the default one.
func loadControlBundle(store *controlplane.FileControlState, defaultID string) (map[string]controlplane.ControlState, error) {
	bundle, err := store.LoadBundle()
	if err != nil {
		return nil, err
	}
	if legacy, ok := bundle[""]; ok {
		delete(bundle, "")
		bundle[defaultID] = legacy
	}
	if _, ok := bundle[defaultID]; !ok {
		bundle[defaultID] = controlplane.ControlState{}
	}
	return bundle, nil
}

// Close releases projection engines and member store adapters opened for this process.
func (ws *Home) Close() error {
	if ws == nil {
		return nil
	}
	ws.closeOnce.Do(func() {
		var first error
		if ws.Projection != nil {
			ws.Projection.Close()
		}
		if ws.ReadCache != nil {
			if err := ws.ReadCache.Close(); err != nil {
				first = err
			}
		}
		if ws.Index != nil {
			if err := ws.Index.Close(); err != nil {
				first = err
			}
		}
		if ws.Store != nil {
			if err := ws.Store.Close(); err != nil && first == nil {
				first = err
			}
		}
		for _, registry := range ws.Registries {
			if err := registry.Close(); err != nil && first == nil {
				first = err
			}
		}
		ws.closeErr = first
	})
	return ws.closeErr
}

func PersistControl(ws *Home) error {
	ws.controlMu.Lock()
	defer ws.controlMu.Unlock()
	if ws.Controls == nil {
		ws.Controls = map[string]controlplane.ControlState{}
	}
	ws.Controls[ws.controlID] = ws.Control
	return ws.ControlStore.SaveBundle(ws.Controls)
}

// BindControl points Control at one Catalog's slice of control.json, filling in
// the maps so verbs can assign without nil checks. The serve facade runs verbs
// concurrently (HTTP requests and the projection worker share one Home), so the
// control binding is serialized; it is process state, never persisted alone.
func (ws *Home) BindControl(catalogID string) {
	ws.controlMu.Lock()
	defer ws.controlMu.Unlock()
	if catalogID == "" {
		catalogID = ws.File.Catalogs[0].ID
	}
	st, ok := ws.Controls[catalogID]
	if !ok {
		st = controlplane.ControlState{}
	}
	if st.Proposals == nil {
		st.Proposals = map[string]controlplane.Proposal{}
	}
	if st.Previews == nil {
		st.Previews = map[string]controlplane.Preview{}
	}
	if st.Validations == nil {
		st.Validations = map[string]controlplane.ValidationReport{}
	}
	ws.Controls[catalogID] = st
	ws.Control = st
	ws.controlID = catalogID
}

func (ws *Home) UseCatalog(catalogID string) (*catalog.Catalog, *catalog.Registry, error) {
	if catalogID == "" {
		return ws.Catalog, ws.Registry, nil
	}
	cat, ok := ws.Catalogs[catalogID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown catalog %s", catalogID)
	}
	return cat, ws.Registries[catalogID], nil
}

// AddCatalog creates a second Catalog in an open home. It normalizes the id here
// rather than in the verb so `catalog-add acme/x` and `init --catalog acme/x`
// cannot disagree about whether the stored id carries the kr:// scheme. Returns
// the id as stored.
func AddCatalog(ws *Home, catalogID string) (string, error) {
	catalogID, err := NormalizeCatalogID(catalogID)
	if err != nil {
		return "", err
	}
	for _, item := range ws.File.Catalogs {
		if item.ID == catalogID {
			return "", fmt.Errorf("catalog already exists: %s", catalogID)
		}
	}
	// One id cannot name both a registry and a Knowledge Repository, or discovery
	// could not tell which kind of directory it found.
	for _, r := range ws.File.Repos {
		if r.ID == catalogID {
			return "", fmt.Errorf("%s is already a member repository", catalogID)
		}
	}
	dir := catalogDirOf(ws.Stores, catalogID)
	registry, err := catalog.NewRegistry(filepath.Join(ws.Dir, dir), catalogID)
	if err != nil {
		return "", err
	}
	cat, err := catalog.NewCatalog(ws.Store, registry)
	if err != nil {
		return "", err
	}
	cat.SetJournal(ws.Journal)
	cat.SetStamp(ws.Writer.Stamp())
	ws.attachIndex(cat)
	if err := cat.RecordCreated(); err != nil {
		return "", err
	}
	ws.File.Catalogs = append(ws.File.Catalogs, HomeCatalog{ID: catalogID, Dir: dir})
	sortCatalogs(ws.File.Catalogs)
	ws.Catalogs[catalogID] = cat
	ws.Registries[catalogID] = registry
	return catalogID, nil
}

// recordCatalogCreated writes the first commit of a brand-new registry. It uses
// a throwaway Store because no member is attached yet at init time.
func recordCatalogCreated(dir, catalogID string, j journal.Journal) error {
	registry, err := catalog.NewRegistry(dir, catalogID)
	if err != nil {
		return err
	}
	cat, err := catalog.NewCatalog(snapshot.NewRegistry(), registry)
	if err != nil {
		return err
	}
	cat.SetJournal(j)
	return cat.RecordCreated()
}

func systemJournalPath(dir string) string { return filepath.Join(dir, "system.jsonl") }
