package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval/opensearch"
	"kc/snapshot"
)

// Generic attachment orchestration. Concrete authority creation, validation,
// discovery and configuration belong only to authority_drivers.go.
//
// The ladder itself is docs/STORE_ADAPTERS.md. Refusals are as load-bearing as
// the constructors: derived stores and dynamic runtimes are not repositories.

// rejectNonRepository turns the known category errors into their own message.
// Each of these is somebody reaching for the wrong layer, not a typo.
func rejectNonRepository(driver string) error {
	switch driver {
	case "mysql":
		return errUnsupportedDriver("repository", driver)
	case "postgres":
		return errUnsupportedDriver("repository", driver)
	}
	return nil
}

// repoDir is where a locally-backed Repository lives under --home.
func repoDir(stores StoresFile, repositoryID string) string {
	root := stores.Layout.Repos
	if root == "" {
		root = defaultReposDir
	}
	return filepath.ToSlash(filepath.Join(root, EncodeRepoDir(repositoryID)))
}

// repoAddRequest is one repo-add invocation.
type repoAddRequest struct {
	ID     string
	Driver string
	DSN    string
	Dir    string
	Link   string
}

// attachRepository resolves the driver, opens the store, and adds it to the live
// Store Directory. It does not touch the Catalog: attaching is ⓪, registering is ①.
func (ws *Home) attachRepository(spec repoAddRequest) (snapshot.Store, error) {
	repositoryID := spec.ID
	if repositoryID == string(knowledge.SystemRepositoryID) {
		return nil, kernel.Fail(kernel.ErrForbidden, "%s is the immutable built-in System Repository", repositoryID)
	}
	for _, item := range ws.File.Catalogs {
		if repositoryID == item.ID {
			return nil, fmt.Errorf("%s is reserved for a Catalog registry", repositoryID)
		}
	}
	for _, r := range ws.File.Repos {
		if r.ID == repositoryID {
			return nil, fmt.Errorf("repository already registered: %s", repositoryID)
		}
	}
	driver := spec.Driver
	if driver == "" {
		driver = ws.Stores.Repository
	}
	driver = normalizeRepoDriver(driver)
	if err := rejectNonRepository(driver); err != nil {
		return nil, err
	}
	authority, err := authorityFor(driver)
	if err != nil {
		return nil, err
	}
	if spec.Dir != "" && spec.Link != "" {
		return nil, fmt.Errorf("use only one of --dir or --link")
	}
	if authority.prepare == nil {
		return nil, fmt.Errorf("repository driver %s cannot create repositories", driver)
	}
	item, err := authority.prepare(ws.Stores, spec)
	if err != nil {
		return nil, err
	}

	repo, err := openAttachedRepository(ws.Dir, item, ws.Stores)
	if err != nil {
		return nil, err
	}
	if err := ws.Store.Add(repo); err != nil {
		return nil, err
	}
	abs, err := resolveStoreDir(ws.Dir, item.Dir, item.Dir)
	if err != nil {
		return nil, err
	}
	if err := stampAuthority(abs, item); err != nil {
		return nil, err
	}
	if filepath.IsAbs(item.Dir) {
		pointerRel := repoDir(ws.Stores, repositoryID)
		pointerAbs, resolveErr := resolveStoreDir(ws.Dir, pointerRel, pointerRel)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if filepath.Clean(pointerAbs) != filepath.Clean(abs) {
			if err := ensureRepositoryPointer(pointerAbs, abs); err != nil {
				return nil, err
			}
			item.Dir = pointerRel
		}
	}
	ws.File.Repos = append(ws.File.Repos, item)
	return repo, nil
}

// An authority opened with --dir must remain discoverable on the next
// process. The home keeps only a filesystem pointer under its configured repo
// root; the external Dolt directory remains the authority and is never copied.
func ensureRepositoryPointer(pointer, target string) error {
	if err := os.MkdirAll(filepath.Dir(pointer), 0o755); err != nil {
		return err
	}
	if existing, err := os.Readlink(pointer); err == nil {
		resolved := existing
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(pointer), resolved)
		}
		if filepath.Clean(resolved) == filepath.Clean(target) {
			return nil
		}
		return fmt.Errorf("repository pointer %s already targets %s", pointer, existing)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("repository path %s already exists", pointer)
	}
	return os.Symlink(target, pointer)
}

func absStoreDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[2:])
	}
	return filepath.Abs(dir)
}

func looksLikeLocalPath(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || strings.Contains(dsn, "://") {
		return false
	}
	return strings.HasPrefix(dsn, "/") || strings.HasPrefix(dsn, ".") || strings.HasPrefix(dsn, "~") || !strings.Contains(dsn, ":")
}

// AddRepository attaches (⓪) and then registers in the default Catalog (①).
func AddRepository(ws *Home, repositoryID, driver, dsn, dir, link string) (kernel.CommitID, error) {
	spec := repoAddRequest{ID: repositoryID, Driver: driver, DSN: dsn, Dir: dir, Link: link}
	repo, err := ws.attachRepository(spec)
	if err != nil {
		return "", err
	}
	if ws.Catalog != nil {
		if err := ws.Catalog.RegisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
			return "", err
		}
	}
	return repo.Head(snapshot.DefaultRef)
}

func openAttachedRepository(home string, repo HomeRepo, stores StoresFile) (snapshot.Store, error) {
	dir := repo.Dir
	if dir == "" {
		dir = repoDir(stores, repo.ID)
	}
	abs, err := resolveStoreDir(home, dir, dir)
	if err != nil {
		return nil, err
	}
	driver := normalizeRepoDriver(repo.Driver)
	if err := rejectNonRepository(driver); err != nil {
		return nil, err
	}
	return openAuthority(abs, repo)
}

// indexOpener picks the ③ projection engine. Errors are deferred into the opener
// so an unusable index driver fails when something actually indexes, not at Open:
// reads that never touch the index keep working.
func indexOpener(file HomeFile, stores StoresFile) index.EngineOpener {
	driver := stores.Index
	if driver == "" {
		driver = file.IndexDriver
	}
	if driver == "" {
		driver = os.Getenv("KC_INDEX_DRIVER")
	}
	driver = normalizeIndexDriver(driver)
	refuse := func(err error) index.EngineOpener {
		return func(string, kernel.RepositoryID) (index.Engine, error) { return nil, err }
	}
	switch driver {
	case "none":
		return refuse(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SEARCH requires an OpenSearch projection; configure --index opensearch"))
	case "opensearch":
		return opensearch.Open(stores.OpenSearch)
	default:
		return refuse(fmt.Errorf("unknown index driver %s", driver))
	}
}
