package cli

// This file is the sole production composition root for concrete Snapshot
// authorities. No Reader, Writer, Catalog, verb, or generic test imports an
// adapter package.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"kc/kernel"
	knowledgedolt "kc/knowledge/dolt"
	"kc/snapshot"
	snapshotdolt "kc/snapshot/dolt"
	"kc/snapshot/gitea"
)

type authorityDriver struct {
	open      func(string, HomeRepo) (snapshot.Store, error)
	discover  func(string, string) (HomeRepo, bool)
	validate  func(HomeRepo) error
	stamp     func(string, HomeRepo) error
	prepare   func(StoresFile, repoAddRequest) (HomeRepo, error)
	configure func(*StoresFile, storeEndpoint) error
	secretEnv string
}

var authorityDrivers = map[string]authorityDriver{
	"dolt": {
		open: func(abs string, item HomeRepo) (snapshot.Store, error) {
			return knowledgedolt.Open(abs, kernel.RepositoryID(item.ID))
		},
		discover: func(home, abs string) (HomeRepo, bool) {
			id, err := snapshotdolt.ReadDoltStamp(abs)
			if err != nil || id == "" {
				return HomeRepo{}, false
			}
			return HomeRepo{ID: string(id), Dir: homeRel(home, abs), Driver: "dolt"}, true
		},
		prepare: func(stores StoresFile, spec repoAddRequest) (HomeRepo, error) {
			if spec.Link != "" {
				return HomeRepo{}, fmt.Errorf("dolt repo-add does not support --link")
			}
			dir := spec.Dir
			if dir == "" && strings.TrimSpace(spec.DSN) != "" {
				if !looksLikeLocalPath(spec.DSN) {
					return HomeRepo{}, fmt.Errorf("--dsn is not used for driver dolt")
				}
				dir = spec.DSN
			}
			item := HomeRepo{ID: spec.ID, Dir: repoDir(stores, spec.ID), Driver: "dolt"}
			if dir != "" {
				abs, err := absStoreDir(dir)
				if err != nil {
					return HomeRepo{}, err
				}
				item.Dir = abs
			}
			return item, nil
		},
		configure: func(file *StoresFile, endpoint storeEndpoint) error {
			file.Repository = "dolt"
			if endpoint.dir != "" {
				file.Layout.Repos = endpoint.dir
			}
			return nil
		},
	},
	"gitea": {
		open: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return gitea.Open(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(gitea.EnvToken))
		},
		discover: func(home, abs string) (HomeRepo, bool) {
			id, dsn, err := gitea.ReadStamp(abs)
			if err != nil || id == "" {
				return HomeRepo{}, false
			}
			return HomeRepo{ID: id, Dir: homeRel(home, abs), Driver: "gitea", DSN: dsn}, true
		},
		validate: func(item HomeRepo) error {
			if strings.TrimSpace(item.DSN) == "" {
				return fmt.Errorf("gitea repository %s is missing dsn", item.ID)
			}
			return snapshot.RejectConfiguredSecret("gitea", item.DSN, gitea.EnvToken)
		},
		stamp: func(abs string, item HomeRepo) error {
			return gitea.WriteStamp(abs, item.ID, item.DSN)
		},
		prepare: func(stores StoresFile, spec repoAddRequest) (HomeRepo, error) {
			if spec.Dir != "" {
				return HomeRepo{}, fmt.Errorf("gitea repo-add does not support --dir")
			}
			dsn := strings.TrimSpace(spec.DSN)
			if dsn == "" {
				dsn = strings.TrimSpace(spec.Link)
			}
			if dsn == "" {
				return HomeRepo{}, fmt.Errorf("gitea repo-add requires --dsn http(s)://host/owner/name")
			}
			return HomeRepo{ID: spec.ID, Dir: repoDir(stores, spec.ID), Driver: "gitea", DSN: dsn}, nil
		},
		configure: func(file *StoresFile, _ storeEndpoint) error {
			file.Repository = "gitea"
			return nil
		},
		secretEnv: gitea.EnvToken,
	},
}

func authorityDriverNames() []string {
	names := make([]string, 0, len(authorityDrivers))
	for name := range authorityDrivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func authoritySecretEnvs() map[string]string {
	out := map[string]string{}
	for name, driver := range authorityDrivers {
		if driver.secretEnv != "" {
			out[name] = driver.secretEnv
		}
	}
	return out
}

func authorityFor(name string) (authorityDriver, error) {
	name = normalizeRepoDriver(name)
	if name == "filegit" {
		return authorityDriver{}, kernel.Fail(kernel.ErrUsageInvalid,
			"repository driver filegit is no longer supported; choose dolt or gitea")
	}
	driver, ok := authorityDrivers[name]
	if !ok {
		return authorityDriver{}, fmt.Errorf("unknown repository driver %s", name)
	}
	return driver, nil
}

func openAuthority(abs string, item HomeRepo) (snapshot.Store, error) {
	driver, err := authorityFor(item.Driver)
	if err != nil {
		return nil, err
	}
	if driver.validate != nil {
		if err := driver.validate(item); err != nil {
			return nil, err
		}
	}
	return driver.open(abs, item)
}

func stampAuthority(abs string, item HomeRepo) error {
	driver, err := authorityFor(item.Driver)
	if err != nil {
		return err
	}
	if driver.stamp == nil {
		return nil
	}
	return driver.stamp(abs, item)
}

func discoverAuthority(home, abs string) (HomeRepo, bool) {
	for _, name := range authorityDriverNames() {
		if item, ok := authorityDrivers[name].discover(home, abs); ok {
			return item, true
		}
	}
	return HomeRepo{}, false
}
