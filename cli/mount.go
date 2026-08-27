package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kc/index"
	"kc/kernel"
	knowledgedolt "kc/knowledge/dolt"
	"kc/retrieval/opensearch"
	"kc/snapshot"
	"kc/snapshot/filegit"
	"kc/snapshot/gitea"
)

// Which engine backs an attached Repository or a projection. This is the one file that
// has to change to add a ⓪ Snapshot adapter or a ③ index engine, so keep the
// driver names, the refusals, and the constructors together here.
//
// The ladder itself is docs/STORE_ADAPTERS.md. Refusals are as load-bearing as
// the constructors: derived stores and dynamic runtimes are not repositories.

// snapshotDrivers is every driver that can back a Knowledge Repository.
var snapshotDrivers = []string{"filegit", "dolt", "gitea"}

func supportedRepositoryDriver(driver string) bool {
	for _, ok := range snapshotDrivers {
		if driver == ok {
			return true
		}
	}
	return false
}

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

const repoLinkFile = ".kc-link"

type repoLink struct {
	ID  string `json:"id"`
	Dir string `json:"dir"`
}

// attachRepository resolves the driver, opens the store, and adds it to the live
// Store Directory. It does not touch the Catalog: attaching is ⓪, registering is ①.
func (ws *Home) attachRepository(spec repoAddRequest) (snapshot.Store, error) {
	repositoryID := spec.ID
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
	if !supportedRepositoryDriver(driver) {
		return nil, fmt.Errorf("unknown repository driver %s", driver)
	}
	if spec.Dir != "" && spec.Link != "" {
		return nil, fmt.Errorf("use only one of --dir or --link")
	}
	if driver == "gitea" && spec.Link != "" && spec.DSN == "" {
		spec.DSN = spec.Link
		spec.Link = ""
	}

	item := HomeRepo{ID: repositoryID, Driver: driver, Dir: repoDir(ws.Stores, repositoryID)}
	switch {
	case spec.Dir != "":
		abs, err := absExistingGit(spec.Dir)
		if err != nil {
			return nil, err
		}
		if err := writeRepoLink(ws.Dir, item.Dir, repositoryID, abs); err != nil {
			return nil, err
		}
		item.Dir = abs
	case spec.Link != "":
		abs, err := resolveStoreDir(ws.Dir, item.Dir, item.Dir)
		if err != nil {
			return nil, err
		}
		if err := cloneGit(spec.Link, abs); err != nil {
			return nil, err
		}
		item.DSN = spec.Link
	case driver == "gitea":
		if strings.TrimSpace(spec.DSN) == "" {
			return nil, fmt.Errorf("gitea repo-add requires --dsn http(s)://host/owner/name")
		}
		if err := snapshot.RejectConfiguredSecret("gitea", spec.DSN, gitea.EnvToken); err != nil {
			return nil, err
		}
		item.DSN = spec.DSN
	}

	repo, err := openAttachedRepository(ws.Dir, item, ws.Stores)
	if err != nil {
		return nil, err
	}
	if err := ws.Store.Add(repo); err != nil {
		return nil, err
	}
	if driver == "gitea" {
		abs, err := resolveStoreDir(ws.Dir, item.Dir, item.Dir)
		if err != nil {
			return nil, err
		}
		if err := gitea.WriteStamp(abs, repositoryID, item.DSN); err != nil {
			return nil, err
		}
	}
	ws.File.Repos = append(ws.File.Repos, item)
	return repo, nil
}

func absExistingGit(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[2:])
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return "", fmt.Errorf("%s is not a git directory", abs)
	}
	return abs, nil
}

func writeRepoLink(home, pointerRel, repositoryID, absDir string) error {
	pointer, err := resolveStoreDir(home, pointerRel, pointerRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pointer, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(repoLink{ID: repositoryID, Dir: absDir})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pointer, repoLinkFile), append(raw, '\n'), 0o644)
}

func readRepoLink(abs string) (repoLink, bool) {
	raw, err := os.ReadFile(filepath.Join(abs, repoLinkFile))
	if err != nil {
		return repoLink{}, false
	}
	var link repoLink
	if json.Unmarshal(raw, &link) != nil || link.ID == "" || link.Dir == "" {
		return repoLink{}, false
	}
	return link, true
}

func cloneGit(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("clone destination %s already exists", dest)
	}
	cmd := exec.Command("git", "clone", "--quiet", url, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git clone: %s", msg)
	}
	return nil
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
	trimmedDSN := strings.TrimSpace(dsn)
	norm := normalizeRepoDriver(driver)
	if spec.Dir == "" && spec.Link == "" && trimmedDSN != "" && norm != "gitea" && looksLikeLocalPath(trimmedDSN) {
		spec.Dir = trimmedDSN
		spec.DSN = ""
		trimmedDSN = ""
	}
	if trimmedDSN != "" && norm != "gitea" {
		if err := applyDSN(&ws.Stores, driver, dsn); err != nil {
			return "", err
		}
		if err := WriteStores(ws.Dir, ws.Stores); err != nil {
			return "", err
		}
	}
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
	id := kernel.RepositoryID(repo.ID)
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
	switch driver {
	case "filegit":
		if managedRepoDir(home, stores, abs) {
			return filegit.NewFileGit(abs, id)
		}
		return filegit.AttachGit(abs, id)
	case "dolt":
		return knowledgedolt.Open(abs, id)
	case "gitea":
		if strings.TrimSpace(repo.DSN) == "" {
			return nil, fmt.Errorf("gitea repository %s is missing dsn", repo.ID)
		}
		return gitea.Open(id, repo.DSN, os.Getenv(gitea.EnvToken))
	default:
		return nil, fmt.Errorf("unknown repository driver %s", repo.Driver)
	}
}

func managedRepoDir(home string, stores StoresFile, abs string) bool {
	root, err := resolveStoreDir(home, stores.Layout.Repos, defaultReposDir)
	if err != nil {
		return false
	}
	realAbs := canonicalPathWithMissingTail(abs)
	realRoot := canonicalPathWithMissingTail(root)
	rel, err := filepath.Rel(realRoot, realAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// canonicalPathWithMissingTail resolves aliases in the longest existing
// prefix and then restores path elements that do not exist yet. EvalSymlinks
// alone is insufficient while repo-add is deciding whether a new destination
// belongs below the managed root: on macOS a temp path can be spelled /var/...
// while its existing parent resolves to /private/var/....
func canonicalPathWithMissingTail(value string) string {
	clean := filepath.Clean(value)
	current := clean
	missing := make([]string, 0, 2)
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return clean
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
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
