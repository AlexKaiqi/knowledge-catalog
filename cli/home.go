package cli

import (
	"fmt"
	"path/filepath"

	"kc/catalog"
	"kc/controlplane"
	"kc/index"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
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
// is home_mount.go; per-verb behaviour is the verbs_*.go files.

type Home struct {
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
	Stores       StoresFile
}

func Open(home string) (*Home, error) {
	file, err := ReadHome(home)
	if err != nil {
		return nil, err
	}
	if len(file.Catalogs) == 0 {
		return nil, fmt.Errorf("no catalog at %s; run: kc local init --home %s", home, home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return nil, err
	}
	if err := stores.validateProfile(); err != nil {
		return nil, err
	}
	store, err := openMembers(home, file, stores)
	if err != nil {
		return nil, err
	}
	catalogs, registries, err := openCatalogs(home, file, store)
	if err != nil {
		return nil, err
	}
	defaultID := file.Catalogs[0].ID
	commands, err := commandlog.New(commandlog.NewBoltStore(
		filepath.Join(home, "writer.db"), filepath.Join(home, "writer.json"),
	))
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
	sys := journal.NewFile(systemPath(home))
	w.SetJournal(sys)
	tw.SetJournal(sys)
	rd := reader.NewReader(store)
	rd.SetJournal(sys)
	for _, cat := range catalogs {
		cat.SetJournal(sys)
	}
	plane := controlplane.New(store, w, catalogs[defaultID])
	plane.SetJournal(sys)
	idxDir, err := resolveStoreDir(home, stores.Layout.Projections, defaultProjectionsDir)
	if err != nil {
		return nil, err
	}
	ws := &Home{
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
	if stores.Index != "none" {
		controller, err := index.NewController(
			ws.Index,
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

// openCatalogs builds every Catalog over the same Store, so a Repository registered in
// two Catalogs is one attached Store, not two.
func openCatalogs(home string, file HomeFile, store *snapshot.Registry) (map[string]*catalog.Catalog, map[string]*catalog.Registry, error) {
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
		bundle[defaultID] = controlplane.EmptyControlState
	}
	return bundle, nil
}

// Close releases projection engines and member store adapters opened for this process.
func (ws *Home) Close() error {
	if ws == nil {
		return nil
	}
	var first error
	if ws.Projection != nil {
		ws.Projection.Close()
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
	return first
}

func PersistControl(ws *Home) error {
	if ws.Controls == nil {
		ws.Controls = map[string]controlplane.ControlState{}
	}
	ws.Controls[ws.controlID] = ws.Control
	return ws.ControlStore.SaveBundle(ws.Controls)
}

// bindControl points Control at one Catalog's slice of control.json, filling in
// the maps so verbs can assign without nil checks.
func (ws *Home) bindControl(catalogID string) {
	if catalogID == "" {
		catalogID = ws.File.Catalogs[0].ID
	}
	st, ok := ws.Controls[catalogID]
	if !ok {
		st = controlplane.EmptyControlState
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
