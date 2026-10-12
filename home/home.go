package home

import (
	"fmt"
	"path/filepath"
	"sync"

	"kc/controlplane"
	"kc/index"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	readcache "kc/retrieval/cache"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// Assembling the live object graph for one command: mount the members, build the
// registry, then hang Writer / Reader / ControlPlane / index off them. This is
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
	Store        snapshot.Registry
	Commands     snapshot.CommandLedger
	Writer       *writer.Writer
	TreeWriter   snapshot.RawWriter
	Reader       *reader.Reader
	Datasets     dataset.Service
	Registry     dataset.Registry
	ControlPlane *controlplane.ControlPlane
	ControlStore *controlplane.FileControlState
	File         HomeFile
	Control      controlplane.ControlState
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
		return nil, err
	}
	return assemble(home, file, stores, store, datasets, registry, false)
}

func assemble(home string, file HomeFile, stores StoresFile, store snapshot.Registry, datasets dataset.Service, registry dataset.Registry, recovering bool) (*Home, error) {
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
	controlStore := controlplane.NewFileControlState(filepath.Join(home, "control.json"), file.Registry.ID)
	control, err := controlStore.Load()
	if err != nil {
		return nil, err
	}
	sys := journal.NewFile(systemJournalPath(home))
	w.SetJournal(sys)
	tw.SetObserver(rawWriteObserver(sys))
	rd := reader.NewReader(store)
	rd.SetJournal(sys)
	datasets.SetObserver(datasetObserver(sys))
	plane := controlplane.New(store, w, datasets)
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
		Datasets:     datasets,
		Registry:     registry,
		ControlPlane: plane,
		ControlStore: controlStore,
		File:         file,
		Control:      control,
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
			if err := controller.RegisterConsumer(datasetProjectionConsumer{datasets: datasets, controller: controller}); err != nil {
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
		if repo.ID == string(knowledge.SystemRepositoryID) {
			opened, err = openExistingAuthority(repo)
		} else {
			opened, err = openAttachedRepository(home, repo, stores)
		}
		if err != nil {
			return nil, err
		}
		if repo.ID == string(knowledge.SystemRepositoryID) {
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
	system, ok := store.Get(knowledge.SystemRepositoryID)
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
	if err := datasets.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
		_ = registry.Close()
		return nil, nil, err
	}
	return datasets, registry, nil
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
		if ws.Registry != nil {
			if err := ws.Registry.Close(); err != nil && first == nil {
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
	return ws.ControlStore.Save(ws.Control)
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
