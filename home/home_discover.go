package home

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"kc/internal/journal"
)

// What is in this --home, found by scanning. There is deliberately no manifest
// listing registry and repositories: the directories are the truth, so a hand-copied or
// half-deleted home cannot disagree with a registry file about what exists.
//
// Registry and Repository directories hold only explicit remote authority
// pointers; accepted Registry content remains in its lakeFS Snapshot.
// That is why `.kc/workspace.json` is not part of the current layout.
//
// This file changes when the on-disk layout changes. Repository adapter
// selection is in home_mount.go; assembling the live object graph is in home.go.

type HomeFile struct {
	Registry    HomeRegistry `json:"registry"`
	Repos       []HomeRepo   `json:"repos"`
	IndexDriver string       `json:"indexDriver,omitempty"`
	IndexURL    string       `json:"indexURL,omitempty"`
}

type HomeRegistry struct {
	ID  string `json:"id"`
	Dir string `json:"dir"`
}

type HomeRepo struct {
	ID     string `json:"id"`
	Dir    string `json:"dir,omitempty"`
	Driver string `json:"driver,omitempty"`
	DSN    string `json:"dsn,omitempty"`
}

func Missing(home string) error {
	return fmt.Errorf("no component fixture at %s; production requires kc deployment init --config", home)
}

// homeReady is "has kc ever written here", answered without parsing anything.
func Ready(home string) bool {
	if _, err := os.Stat(LayoutPath(home)); err == nil {
		return true
	}
	if _, err := os.Stat(StoresPath(home)); err == nil {
		return true
	}
	return false
}

func registryDir(stores StoresFile) string {
	if stores.Layout.Registry != "" {
		return stores.Layout.Registry
	}
	return defaultRegistryDir
}

func homeRel(home, abs string) string {
	rel, err := filepath.Rel(home, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

var unsafeRepoChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// EncodeRepoDir maps a kr:// id to one directory name. Lossy on purpose: the
// authoritative id is stamped inside the directory, never read back off the path.
func EncodeRepoDir(repositoryID string) string {
	return unsafeRepoChars.ReplaceAllString(repositoryID, "_")
}

// InitHome creates an explicit component registry; production uses deployment init.
func InitHome(home string, binding RegistryBinding) (HomeFile, bool, error) {
	id, err := NormalizeRepositoryID(binding.ID)
	if err != nil || id != binding.ID {
		return HomeFile{}, false, fmt.Errorf("registry requires an exact Snapshot Repository identity")
	}
	stores := DefaultStores()
	created := !Ready(home)
	if created {
		if err := WriteStores(home, stores); err != nil {
			return HomeFile{}, false, err
		}
	}
	stores, err = ReadStores(home)
	if err != nil {
		return HomeFile{}, false, err
	}
	file, err := ReadHome(home)
	if err != nil {
		return HomeFile{}, false, err
	}
	if file.Registry.ID != "" {
		if file.Registry.ID != id {
			return HomeFile{}, false, fmt.Errorf("this home already has registry %s", file.Registry.ID)
		}
		return file, false, nil
	}
	abs, err := ResolveStoreDir(home, registryDir(stores), defaultRegistryDir)
	if err != nil {
		return HomeFile{}, false, err
	}
	if err := recordRegistryCreated(abs, journal.NewFile(systemJournalPath(home)), binding); err != nil {
		return HomeFile{}, false, err
	}
	file, err = ReadHome(home)
	return file, true, err
}

func ReadHome(home string) (HomeFile, error) {
	if !Ready(home) {
		return HomeFile{}, Missing(home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return HomeFile{}, err
	}
	return discoverHome(home, stores)
}

// discoverHome reads the one registry pointer and discovers source pointers.
func discoverHome(home string, stores StoresFile) (HomeFile, error) {
	abs, err := ResolveStoreDir(home, registryDir(stores), defaultRegistryDir)
	if err != nil {
		return HomeFile{}, err
	}
	out := HomeFile{}
	if binding, err := readFixtureRegistryBinding(abs); err == nil {
		out.Registry = HomeRegistry{ID: binding.ID, Dir: homeRel(home, abs)}
	} else if !os.IsNotExist(err) {
		return HomeFile{}, err
	}
	reposByID := discoverRepos(home, stores, map[string]bool{abs: true})
	for _, r := range reposByID {
		out.Repos = append(out.Repos, r)
	}
	sort.Slice(out.Repos, func(i, j int) bool { return out.Repos[i].ID < out.Repos[j].ID })
	return out, nil
}

func discoverRepos(home string, stores StoresFile, registryAbs map[string]bool) map[string]HomeRepo {
	found := map[string]HomeRepo{}
	seen := map[string]bool{}
	scan := func(dir, fallback string) {
		root, err := ResolveStoreDir(home, dir, fallback)
		if err != nil || seen[root] {
			return
		}
		seen[root] = true
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			isRepositoryEntry := e.IsDir() || e.Type()&os.ModeSymlink != 0
			if !isRepositoryEntry {
				continue
			}
			abs := filepath.Join(root, e.Name())
			if registryAbs[abs] {
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

// peekRepoDir reads the id a directory stamped on itself. lakeFS keeps a remote stamp only.
func peekRepoDir(home, abs string) (HomeRepo, bool) {
	if _, err := readFixtureRegistryBinding(abs); err == nil {
		return HomeRepo{}, false
	}
	return discoverAuthority(home, abs)
}
