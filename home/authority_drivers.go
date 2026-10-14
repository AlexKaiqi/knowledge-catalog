package home

// This file is the sole production composition root for concrete Snapshot
// authorities. No Reader, Writer, Registry, verb, or generic test imports an
// adapter package.

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

type authorityDriver struct {
	connectionOpen  func(RepositoryBinding, string, string) (snapshot.Store, string, error)
	openExisting    func(string, HomeRepo) (snapshot.Store, error)
	open            func(string, HomeRepo) (snapshot.Store, error)
	discover        func(string, string) (HomeRepo, bool)
	validate        func(HomeRepo) error
	stamp           func(string, HomeRepo) error
	prepare         func(StoresFile, repoAddRequest) (HomeRepo, error)
	configure       func(*StoresFile, storeEndpoint) error
	secretEnv       string
	managedValidate func(ManagedRepositoryConfig, DeploymentConfig) error
	managedBinding  func(ManagedRepositoryConfig, ManagedRepositoryRequest, string) (RepositoryBinding, error)
	managedURL      func(ManagedRepositoryConfig, RepositoryBinding) string
	managedCreate   func(ManagedRepositoryConfig, RepositoryBinding, string) (snapshot.Store, string, error)
	managedOpen     func(RepositoryBinding, string, string) (snapshot.Store, error)
}

var authorityDrivers = map[string]authorityDriver{
	"lakefs": {
		connectionOpen: func(binding RepositoryBinding, credential, expected string) (snapshot.Store, string, error) {
			if strings.TrimSpace(credential) == "" {
				return nil, "", kernel.Fail(kernel.ErrUnauthenticated, "repository connection requires its own credential")
			}
			endpoint, err := snapshot.ParseLakeFSDSN(binding.DSN)
			if err != nil {
				return nil, "", err
			}
			if expected != "" && expected != endpoint.Repository {
				return nil, "", kernel.Fail(kernel.ErrPreconditionFailed, "connected LakeFS authority identity changed")
			}
			repo, err := snapshot.OpenLakeFS(kernel.RepositoryID(binding.ID), binding.DSN, credential)
			return repo, endpoint.Repository, err
		},
		openExisting: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return snapshot.OpenLakeFS(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(snapshot.LakeFSCredentialEnv))
		},
		open: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return snapshot.OpenLakeFS(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(snapshot.LakeFSCredentialEnv))
		},
		discover: func(home, abs string) (HomeRepo, bool) {
			id, dsn, err := snapshot.ReadLakeFSStamp(abs)
			if err != nil || id == "" {
				return HomeRepo{}, false
			}
			return HomeRepo{ID: id, Dir: homeRel(home, abs), Driver: "lakefs", DSN: dsn}, true
		},
		validate: func(item HomeRepo) error {
			if strings.TrimSpace(item.DSN) == "" {
				return fmt.Errorf("lakefs repository %s is missing dsn", item.ID)
			}
			_, err := snapshot.ParseLakeFSDSN(item.DSN)
			return err
		},
		stamp: func(abs string, item HomeRepo) error {
			return snapshot.WriteLakeFSStamp(abs, item.ID, item.DSN)
		},
		prepare: func(stores StoresFile, spec repoAddRequest) (HomeRepo, error) {
			if spec.Dir != "" {
				return HomeRepo{}, fmt.Errorf("lakefs repo-add does not support --dir")
			}
			dsn := strings.TrimSpace(spec.DSN)
			if dsn == "" {
				dsn = strings.TrimSpace(spec.Link)
			}
			if dsn == "" {
				return HomeRepo{}, fmt.Errorf("lakefs repo-add requires --dsn http(s)://host/repository")
			}
			if _, err := snapshot.ParseLakeFSDSN(dsn); err != nil {
				return HomeRepo{}, err
			}
			return HomeRepo{ID: spec.ID, Dir: repoDir(stores, spec.ID), Driver: "lakefs", DSN: dsn}, nil
		},
		configure: func(file *StoresFile, _ storeEndpoint) error {
			file.Repository = "lakefs"
			return nil
		},
		secretEnv: snapshot.LakeFSCredentialEnv,
		managedValidate: func(pool ManagedRepositoryConfig, _ DeploymentConfig) error {
			if strings.TrimSpace(pool.DSN) == "" || strings.ContainsAny(pool.DSN, "?#") {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an origin dsn")
			}
			if _, err := snapshot.ParseLakeFSDSN(strings.TrimRight(pool.DSN, "/") + "/name-probe"); err != nil {
				return err
			}
			namespace, err := url.Parse(strings.TrimSpace(pool.Root))
			if err != nil || namespace.Scheme != "s3" || namespace.Host == "" || namespace.User != nil || namespace.RawQuery != "" || namespace.Fragment != "" {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an s3:// storage namespace prefix in root")
			}
			return nil
		},
		managedBinding: func(pool ManagedRepositoryConfig, req ManagedRepositoryRequest, allocation string) (RepositoryBinding, error) {
			name, err := snapshot.ManagedLakeFSName(req.Name, req.Principal, allocation)
			if err != nil {
				return RepositoryBinding{}, err
			}
			return RepositoryBinding{ID: req.RepositoryID, Driver: "lakefs", DSN: strings.TrimRight(pool.DSN, "/") + "/" + name}, nil
		},
		managedURL: func(pool ManagedRepositoryConfig, binding RepositoryBinding) string {
			if pool.PublicURL == "" {
				return ""
			}
			name := binding.ID
			if ep, err := snapshot.ParseLakeFSDSN(binding.DSN); err == nil && ep.Repository != "" {
				name = ep.Repository
			}
			return strings.TrimRight(pool.PublicURL, "/") + "/repositories/" + url.PathEscape(name)
		},
		managedCreate: func(pool ManagedRepositoryConfig, binding RepositoryBinding, allocation string) (snapshot.Store, string, error) {
			return snapshot.CreateManagedLakeFS(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(snapshot.LakeFSCredentialEnv), allocation, pool.Root)
		},
		managedOpen: func(binding RepositoryBinding, allocation, backend string) (snapshot.Store, error) {
			return snapshot.OpenManagedLakeFS(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(snapshot.LakeFSCredentialEnv), allocation, backend)
		},
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
	driver, ok := authorityDrivers[name]
	if !ok {
		return authorityDriver{}, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository driver %s", name)
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

func openExistingAuthority(item HomeRepo) (snapshot.Store, error) {
	driver, err := authorityFor(item.Driver)
	if err != nil {
		return nil, err
	}
	if driver.validate != nil {
		if err := driver.validate(item); err != nil {
			return nil, err
		}
	}
	return driver.openExisting(item.Dir, item)
}

// openRegistryAuthority opens the independent Registry Snapshot. Registry is not a
// Object Repository: registry authorities open existing remote Snapshots.
func openRegistryAuthority(binding RegistryBinding) (snapshot.Store, error) {
	switch binding.Driver {
	case "lakefs":
		driver, err := authorityFor(binding.Driver)
		if err != nil {
			return nil, err
		}
		if driver.openExisting == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "registry authority driver %s cannot open an existing Snapshot", binding.Driver)
		}
		return driver.openExisting("", binding.homeRepo())
	default:
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "registry %s requires Snapshot driver lakefs", binding.ID)
	}
}

func managedLakeFSName(name, principal, identity string) (string, error) {
	return snapshot.ManagedLakeFSName(name, principal, identity)
}

func validExternalAuthorityName(name string) bool {
	return snapshot.ValidLakeFSName(name) && !snapshot.PlatformLakeFSName(name)
}
