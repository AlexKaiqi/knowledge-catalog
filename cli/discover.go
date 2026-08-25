package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kc/catalog"
	"kc/internal/journal"
	"kc/snapshot/dolt"
	"kc/snapshot/filegit"
	"kc/snapshot/gitea"
)

// What is in this --home, found by scanning. There is deliberately no manifest
// listing catalogs and repos: the directories are the truth, so a hand-copied or
// half-deleted home cannot disagree with a registry file about what exists.
//
// A Catalog is identified by catalog.yaml at HEAD (or the kc.catalogId stamp
// before the first commit); a knowledge Repository by git config kc.repositoryId.
// That is why `.kc/workspace.json` is not part of the current layout.
//
// This file changes when the on-disk layout changes. Which engine backs a mounted
// repo is mount.go; assembling the live object graph is workspace.go.

type HomeFile struct {
	Catalogs    []HomeCatalog `json:"catalogs"`
	Repos       []HomeRepo    `json:"repos"`
	IndexDriver string        `json:"indexDriver,omitempty"`
	IndexURL    string        `json:"indexURL,omitempty"`
}

type HomeCatalog struct {
	ID  string `json:"id"`
	Dir string `json:"dir"`
}

type HomeRepo struct {
	ID     string `json:"id"`
	Dir    string `json:"dir,omitempty"`
	Driver string `json:"driver,omitempty"`
	DSN    string `json:"dsn,omitempty"`
}

func missingHome(home string) error {
	return fmt.Errorf("no kc home at %s; run: kc init --home %s", home, home)
}

// homeReady is "has kc ever written here", answered without parsing anything.
func homeReady(home string) bool {
	if _, err := os.Stat(layoutPath(home)); err == nil {
		return true
	}
	if _, err := os.Stat(storesPath(home)); err == nil {
		return true
	}
	return false
}

func catalogDirOf(stores StoresFile, catalogID string) string {
	root := stores.Layout.Catalogs
	if root == "" {
		root = defaultCatalogsDir
	}
	return filepath.ToSlash(filepath.Join(root, EncodeRepoDir(catalogID)))
}

func homeRel(home, abs string) string {
	rel, err := filepath.Rel(home, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

func isGitDir(abs string) bool {
	_, err := os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}

var unsafeRepoChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// EncodeRepoDir maps a kr:// id to one directory name. Lossy on purpose: the
// authoritative id is stamped inside the directory, never read back off the path.
func EncodeRepoDir(repositoryID string) string {
	return unsafeRepoChars.ReplaceAllString(repositoryID, "_")
}

// NormalizeCatalogID accepts kr://<org>/<name> or <org>/<name>.
func NormalizeCatalogID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("catalog id is required")
	}
	if strings.Contains(raw, "://") {
		if !strings.HasPrefix(raw, "kr://") {
			return "", fmt.Errorf("catalog id must use kr://, got %s", raw)
		}
		rest := strings.TrimPrefix(raw, "kr://")
		if rest == "" || strings.HasSuffix(rest, "/") || !strings.Contains(rest, "/") {
			return "", fmt.Errorf("catalog id must be kr://<org>/<name>, got %s", raw)
		}
		return raw, nil
	}
	if !strings.Contains(raw, "/") {
		return "", fmt.Errorf("catalog id must be kr://<org>/<name> or <org>/<name>, got %q", raw)
	}
	return "kr://" + raw, nil
}

func InitHome(home, catalogID string) (HomeFile, bool, error) {
	stores := DefaultStores()
	repoRoot, err := resolveStoreDir(home, stores.Layout.Repos, defaultReposDir)
	if err != nil {
		return HomeFile{}, false, err
	}
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		return HomeFile{}, false, err
	}
	catRoot, err := resolveStoreDir(home, stores.Layout.Catalogs, defaultCatalogsDir)
	if err != nil {
		return HomeFile{}, false, err
	}
	if err := os.MkdirAll(catRoot, 0o755); err != nil {
		return HomeFile{}, false, err
	}
	createdHome := false
	if !homeReady(home) {
		if err := WriteStores(home, stores); err != nil {
			return HomeFile{}, false, err
		}
		createdHome = true
	}
	stores, err = ReadStores(home)
	if err != nil {
		return HomeFile{}, false, err
	}
	file, err := ReadHome(home)
	if err != nil {
		return HomeFile{}, false, err
	}
	if catalogID != "" {
		catalogID, err = NormalizeCatalogID(catalogID)
		if err != nil {
			return HomeFile{}, false, err
		}
	} else if createdHome || len(file.Catalogs) == 0 {
		catalogID = catalog.DefaultCatalogID
	} else {
		return file, false, nil
	}
	if hasCatalog(file, catalogID) {
		return file, false, nil
	}
	// Re-running init with a different id in a populated home is a typo, not a
	// request for a second Catalog. catalog-add is the explicit verb for that.
	if !createdHome && len(file.Catalogs) > 0 {
		return HomeFile{}, false, fmt.Errorf("this home already has catalog %s; requested %s. Use catalog-add, or a different --home", file.Catalogs[0].ID, catalogID)
	}
	dirRel := catalogDirOf(stores, catalogID)
	catalogAbs, err := resolveStoreDir(home, dirRel, dirRel)
	if err != nil {
		return HomeFile{}, false, err
	}
	if err := recordCatalogCreated(catalogAbs, catalogID, journal.NewFile(systemPath(home))); err != nil {
		return HomeFile{}, false, err
	}
	file, err = ReadHome(home)
	return file, true, err
}

func hasCatalog(file HomeFile, catalogID string) bool {
	for _, item := range file.Catalogs {
		if item.ID == catalogID {
			return true
		}
	}
	return false
}

func ReadHome(home string) (HomeFile, error) {
	if !homeReady(home) {
		return HomeFile{}, missingHome(home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return HomeFile{}, err
	}
	return discoverHome(home, stores)
}

func addCatalogDir(home, abs string, byID map[string]HomeCatalog) {
	if !isGitDir(abs) {
		return
	}
	id, err := catalog.PeekID(abs)
	if err != nil || id == "" {
		id, err = catalog.PeekStamp(abs)
		if err != nil || id == "" {
			return
		}
	}
	if _, ok := byID[id]; ok {
		return
	}
	byID[id] = HomeCatalog{ID: id, Dir: homeRel(home, abs)}
}

func scanCatalogRoot(home, rootAbs string, byID map[string]HomeCatalog) {
	entries, err := os.ReadDir(rootAbs)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		addCatalogDir(home, filepath.Join(rootAbs, e.Name()), byID)
	}
}

func sortCatalogs(catalogs []HomeCatalog) {
	sort.Slice(catalogs, func(i, j int) bool {
		return catalogs[i].ID < catalogs[j].ID
	})
}

// discoverHome walks the configured roots, keyed by the id stamped in each
// directory.
func discoverHome(home string, stores StoresFile) (HomeFile, error) {
	byID := map[string]HomeCatalog{}
	catalogAbs := discoverCatalogs(home, stores, byID)
	reposByID := discoverRepos(home, stores, catalogAbs)

	catalogs := make([]HomeCatalog, 0, len(byID))
	for _, c := range byID {
		catalogs = append(catalogs, c)
	}
	sortCatalogs(catalogs)
	repos := make([]HomeRepo, 0, len(reposByID))
	for _, r := range reposByID {
		repos = append(repos, r)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })

	out := HomeFile{Catalogs: catalogs, Repos: repos}
	return out, nil
}

// discoverCatalogs fills byID and returns the absolute dirs it claimed, so repo
// discovery can skip them: a registry git is not a knowledge Repository.
func discoverCatalogs(home string, stores StoresFile, byID map[string]HomeCatalog) map[string]bool {
	seen := map[string]bool{}
	scan := func(dir, fallback string) {
		abs, err := resolveStoreDir(home, dir, fallback)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		scanCatalogRoot(home, abs, byID)
	}
	scan(stores.Layout.Catalogs, defaultCatalogsDir)
	scan(defaultCatalogsDir, defaultCatalogsDir)
	scan(legacyCatalogsDir, legacyCatalogsDir)
	// Single-catalog layouts point at the registry dir itself, not a parent root.
	if stores.Layout.Catalog != "" {
		if abs, err := resolveStoreDir(home, stores.Layout.Catalog, stores.Layout.Catalog); err == nil {
			addCatalogDir(home, abs, byID)
		}
	}
	if abs, err := resolveStoreDir(home, legacyCatalogDir, legacyCatalogDir); err == nil {
		addCatalogDir(home, abs, byID)
	}

	claimed := map[string]bool{}
	for _, c := range byID {
		if abs, err := resolveStoreDir(home, c.Dir, c.Dir); err == nil {
			claimed[abs] = true
		}
	}
	return claimed
}

func discoverRepos(home string, stores StoresFile, catalogAbs map[string]bool) map[string]HomeRepo {
	found := map[string]HomeRepo{}
	seen := map[string]bool{}
	scan := func(dir, fallback string) {
		root, err := resolveStoreDir(home, dir, fallback)
		if err != nil || seen[root] {
			return
		}
		seen[root] = true
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "_catalog" || e.Name() == "_catalogs" {
				continue
			}
			abs := filepath.Join(root, e.Name())
			if catalogAbs[abs] {
				continue
			}
			item, ok := peekRepoDir(home, abs)
			if !ok {
				continue
			}
			if _, taken := found[item.ID]; taken {
				continue
			}
			found[item.ID] = item
		}
	}
	scan(stores.Layout.Repos, defaultReposDir)
	scan(defaultReposDir, defaultReposDir)
	return found
}

// peekRepoDir reads the id a directory stamped on itself. Gitea has a remote
// stamp only, native Dolt has .dolt + its own stamp, and FileGit carries
// kc.repositoryId in .git/config.
func peekRepoDir(home, abs string) (HomeRepo, bool) {
	if link, ok := readRepoLink(abs); ok {
		return HomeRepo{ID: link.ID, Dir: link.Dir, Driver: "filegit"}, true
	}
	if !isGitDir(abs) {
		if id, err := dolt.ReadDoltStamp(abs); err == nil {
			return HomeRepo{ID: string(id), Dir: homeRel(home, abs), Driver: "dolt"}, true
		}
		id, dsn, err := gitea.ReadStamp(abs)
		if err != nil || id == "" {
			return HomeRepo{}, false
		}
		return HomeRepo{ID: id, Dir: homeRel(home, abs), Driver: "gitea", DSN: dsn}, true
	}
	if _, err := catalog.PeekID(abs); err == nil {
		return HomeRepo{}, false
	}
	id, driver, err := filegit.ReadFileGitStamp(abs)
	if err != nil || id == "" {
		return HomeRepo{}, false
	}
	return HomeRepo{ID: id, Dir: homeRel(home, abs), Driver: driver}, true
}
