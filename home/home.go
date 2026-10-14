package home

import (
	"errors"
	"fmt"
	discoverydatasets "kc/discovery/datasets"
	discoveryplugin "kc/discovery/plugin"
	"path/filepath"
	"sync"

	"kc/internal/journal"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	readcache "kc/object/cache"
	"kc/object/maintenance"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// Assembling the live object graph for one command: mount the members, build the
// registry, then hang Writer / Reader / index off them. This is
// the only place that knows the wiring order, and it changes when the set of
// collaborators changes.
//
// What lives in this --home is home_discover.go; which engine backs a member
// is home_mount.go. Per-verb behaviour stays in cli/verbs_*.go.

type Home struct {
	inventory          *homeInventory
	readOnly           bool
	closeOnce          sync.Once
	closeErr           error
	indexDirectory     string
	pluginsVerified    bool
	liveAccessEntries  map[string]liveservice.PluginEntry
	Deployment         *DeploymentConfig
	Dir                string
	Store              snapshot.Registry
	Commands           snapshot.CommandLedger
	Writer             *writer.Writer
	TreeWriter         snapshot.RawWriter
	Reader             *reader.Reader
	Datasets           dataset.Service
	Registry           dataset.Registry
	File               HomeFile
	Journal            journal.Journal
	Index              discoveryplugin.IndexQueries
	Plugins            *discoveryplugin.IndexPlugins
	LiveAccess         *liveservice.Plugins
	ObjectMaintenance  *maintenance.Coordinator
	DatasetPreparation discoverydatasets.Preparation
	Hydrator           objectmodel.Hydrator
	ReadCache          *readcache.Cache
	Stores             StoresFile
}

func Open(home string) (*Home, error) {
	file, err := ReadHome(home)
	if err != nil {
		return nil, err
	}
	if file.Registry.ID == "" {
		return nil, fmt.Errorf("no registry fixture at %s; production requires kc deployment init --config (fixture %s)", home, home)
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
	datasets, registry, err := OpenDatasets(home, file, store)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	return assemble(home, file, stores, store, datasets, registry, false)
}

// assemble takes ownership of the authority handles even when construction fails.
func assemble(home string, file HomeFile, stores StoresFile, store snapshot.Registry, datasets dataset.Service, registry dataset.Registry, recovering bool) (result *Home, err error) {
	ws := &Home{
		inventory: &homeInventory{bindings: map[string]HomeRepo{}},
		Dir:       home, Store: store, Datasets: datasets, Registry: registry,
		File: file, Stores: stores,
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, ws.Close())
		}
	}()
	var ledgerStore snapshot.CommandStore = snapshot.NewBoltCommandStore(filepath.Join(home, "writer.db"))
	if recovering {
		existing, err := snapshot.OpenBoltCommandStore(filepath.Join(home, "writer.db"))
		if err != nil {
			return nil, err
		}
		ledgerStore = existing
	}
	commands, err := snapshot.NewCommandLedger(ledgerStore)
	if err != nil {
		return nil, err
	}
	w, err := writer.NewWriter(store, commands)
	if err != nil {
		return nil, err
	}
	tw, err := snapshot.NewRawWriter(store, commands)
	if err != nil {
		return nil, err
	}
	sys := journal.NewFile(systemJournalPath(home))
	w.SetJournal(sys)
	tw.SetObserver(rawWriteObserver(sys))
	rd := reader.NewReader(store)
	rd.SetJournal(sys)
	datasets.SetObserver(datasetObserver(sys))
	idxDir, err := ResolveStoreDir(home, stores.Layout.Projections, defaultProjectionsDir)
	if err != nil {
		return nil, err
	}
	ws.Commands, ws.Writer, ws.TreeWriter, ws.Reader = commands, w, tw, rd
	ws.Journal = sys
	for _, binding := range file.Repos {
		ws.inventory.bindings[binding.ID] = binding
	}
	if err := ws.configureHydration(); err != nil {
		return nil, err
	}
	if err := ws.configureIndexPlugins(idxDir); err != nil {
		return nil, err
	}
	if ws.ReadCache != nil {
		controller, err := maintenance.NewCoordinator(
			filepath.Join(idxDir, "object-maintenance.db"),
			func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return rd.Require(id, kernel.ErrCapabilityUnsatisfied)
			},
		)
		if err != nil {
			return nil, err
		}
		controller.SetInventory(func() ([]kernel.RepositoryID, error) {
			return store.IDs(), nil
		})
		ws.ObjectMaintenance = controller
		if ws.ReadCache != nil {
			if err := controller.RegisterConsumer(ws.ReadCache); err != nil {
				return nil, err
			}
		}
	}
	ws.DatasetPreparation = discoverydatasets.Preparation{Enabled: ws.Plugins.Configured(), Provider: ws.Plugins, Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
	}}
	ws.wireSidecars()
	return ws, nil
}

// openMembers opens every discovered Repository. One bad member fails the command:
// a partially attached Store Directory would silently answer Workspace reads with fewer sources.
func openMembers(home string, file HomeFile, stores StoresFile) (snapshot.Registry, error) {
	store := snapshot.NewRegistry()
	openedAll := false
	defer func() {
		if !openedAll {
			_ = store.Close()
		}
	}()
	for _, repo := range file.Repos {
		if repo.DSN != "" {
			if err := snapshot.RejectConfiguredSecret(repo.Driver, repo.DSN, snapshot.LakeFSCredentialEnv); err != nil {
				return nil, fmt.Errorf("repository %s: %w", repo.ID, err)
			}
		}
		var opened snapshot.Store
		var err error
		if repo.ID == string(objectmodel.SystemRepositoryID) {
			opened, err = openExistingAuthority(repo)
		} else {
			opened, err = openAttachedRepository(home, repo, stores)
		}
		if err != nil {
			return nil, err
		}
		if repo.ID == string(objectmodel.SystemRepositoryID) {
			opened, err = readOnlySystemAuthority(opened)
			if err != nil {
				return nil, err
			}
		}
		if err := store.Add(opened); err != nil {
			closeManagedSource(opened)
			return nil, err
		}
	}
	system, ok := store.Get(objectmodel.SystemRepositoryID)
	if !ok {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "component fixture requires a published System Repository")
	}
	if err := verifySystemAuthority(system); err != nil {
		return nil, err
	}
	openedAll = true
	return store, nil
}

func OpenDatasets(home string, file HomeFile, store snapshot.Registry) (dataset.Service, dataset.Registry, error) {
	item := file.Registry
	binding, err := readFixtureRegistryBinding(filepath.Join(home, item.Dir))
	if err != nil {
		return nil, nil, err
	}
	if binding.ID != item.ID {
		return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "component registry pointer changed identity")
	}
	registry, err := openRegistry(binding)
	if err != nil {
		return nil, nil, err
	}
	datasets, err := dataset.NewService(store, registry)
	if err != nil {
		_ = registry.Close()
		return nil, nil, err
	}
	if err := datasets.RegisterRepository(objectmodel.SystemRepositoryID); err != nil {
		_ = registry.Close()
		return nil, nil, err
	}
	return datasets, registry, nil
}

// StopMaintenance joins workers while preserving resources for accepted tasks.
func (ws *Home) StopMaintenance() {
	if ws == nil || ws.readOnly {
		return
	}
	ws.Plugins.StopBackground()
	if ws.ObjectMaintenance != nil {
		ws.ObjectMaintenance.Close()
	}
}

// Close releases projection engines and member store adapters opened for this process.
func (ws *Home) Close() error {
	if ws == nil || ws.readOnly {
		return nil
	}
	ws.closeOnce.Do(func() {
		var first error
		ws.StopMaintenance()
		if ws.Plugins != nil {
			if err := ws.Plugins.Close(); err != nil {
				first = err
			}
		}
		if ws.LiveAccess != nil {
			first = errors.Join(first, ws.LiveAccess.Close())
		}
		if ws.ReadCache != nil {
			if err := ws.ReadCache.Close(); err != nil && first == nil {
				first = err
			}
		}
		if ws.Registry != nil {
			if err := ws.Registry.Close(); err != nil && first == nil {
				first = err
			}
		}
		if ws.Store != nil {
			if err := ws.Store.Close(); err != nil && first == nil {
				first = err
			}
		}
		ws.closeErr = first
	})
	return ws.closeErr
}

func recordRegistryCreated(dir string, j journal.Journal, binding RegistryBinding) error {
	binding, err := fixtureRegistryBinding(binding)
	if err != nil {
		return err
	}
	registry, err := createRegistry(binding)
	if err != nil {
		return err
	}
	defer registry.Close()
	datasets, err := dataset.NewService(snapshot.NewRegistry(), registry)
	if err != nil {
		return err
	}
	datasets.SetObserver(datasetObserver(j))
	if err := datasets.RecordCreated(); err != nil {
		return err
	}
	return writeFixtureRegistryBinding(dir, binding)
}

func systemJournalPath(dir string) string { return filepath.Join(dir, "system.jsonl") }
