package home

// This file is the sole production composition root for concrete Snapshot
// authorities. No Reader, Writer, Catalog, verb, or generic test imports an
// adapter package.

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
	knowledgedolt "kc/knowledge/dolt"
	"kc/snapshot"
	snapshotdolt "kc/snapshot/dolt"
	"kc/snapshot/gitea"
	"kc/snapshot/lakefs"
)

type authorityDriver struct {
	connectionOpen      func(RepositoryBinding, string, string) (snapshot.Store, string, error)
	openExisting        func(string, HomeRepo) (snapshot.Store, error)
	open                func(string, HomeRepo) (snapshot.Store, error)
	discover            func(string, string) (HomeRepo, bool)
	validate            func(HomeRepo) error
	stamp               func(string, HomeRepo) error
	prepare             func(StoresFile, repoAddRequest) (HomeRepo, error)
	configure           func(*StoresFile, storeEndpoint) error
	secretEnv           string
	managedValidate     func(ManagedRepositoryConfig, DeploymentConfig) error
	managedBinding      func(ManagedRepositoryConfig, ManagedRepositoryRequest, string) (RepositoryBinding, error)
	managedURL          func(ManagedRepositoryConfig, RepositoryBinding) string
	managedEnsureOwner  func(managedRecord) (string, error)
	managedTrustedOwner func(ManagedRepositoryRequest, RepositoryBinding) bool
	managedCreate       func(ManagedRepositoryConfig, RepositoryBinding, string, bool) (snapshot.Store, string, error)
	managedOpen         func(RepositoryBinding, string, string) (snapshot.Store, error)
	managedRestore      func(managedRecord) snapshot.Store
}

// mountedDoltFiles is the explicitly named file-capability adapter used only
// by the Home composition root. The native Knowledge provider itself does not
// implement TreeStore; ordinary Repository/VFS flows that intentionally need
// literal files receive this separate adapter at assembly.
type mountedDoltFiles struct {
	*knowledgedolt.Repository
	raw *snapshotdolt.DoltRepository
}

func (s *mountedDoltFiles) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return s.raw.ApplyTreeCommit(change)
}

func (s *mountedDoltFiles) Close() error {
	if err := s.Repository.Close(); err != nil {
		return err
	}
	return s.raw.Close()
}

func mountDoltFiles(native *knowledgedolt.Repository, raw *snapshotdolt.DoltRepository) snapshot.Store {
	return &mountedDoltFiles{Repository: native, raw: raw}
}

var authorityDrivers = map[string]authorityDriver{
	"dolt": {
		managedValidate: func(pool ManagedRepositoryConfig, config DeploymentConfig) error {
			if pool.DSN != "" || !filepath.IsAbs(pool.Root) || pathsOverlap(pool.Root, config.CacheDir) || pathsOverlap(pool.Root, config.StateDir) {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed Dolt requires an absolute durable root independent of cache and control state; dsn is not accepted")
			}
			return nil
		},
		managedBinding: func(pool ManagedRepositoryConfig, req ManagedRepositoryRequest, allocation string) (RepositoryBinding, error) {
			root := pool.Root
			if validManagedUsername(req.Principal) {
				root = filepath.Join(root, req.Principal)
			}
			return RepositoryBinding{ID: req.RepositoryID, Driver: "dolt", Dir: filepath.Join(root, "kc-"+allocation)}, nil
		},
		managedURL: func(pool ManagedRepositoryConfig, binding RepositoryBinding) string {
			if pool.PublicURL == "" {
				return ""
			}
			return strings.TrimRight(pool.PublicURL, "/") + "/repositories/" + url.PathEscape(binding.ID)
		},
		managedCreate: func(_ ManagedRepositoryConfig, binding RepositoryBinding, allocation string, _ bool) (snapshot.Store, string, error) {
			base, err := snapshotdolt.CreateManaged(binding.Dir, kernel.RepositoryID(binding.ID), allocation)
			if err != nil {
				return nil, "", err
			}
			repo, err := knowledgedolt.Wrap(base)
			if err != nil {
				return nil, "", err
			}
			return mountDoltFiles(repo, base), allocation, nil
		},
		managedOpen: func(binding RepositoryBinding, allocation, backend string) (snapshot.Store, error) {
			if backend != allocation {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed Dolt ownership receipt is invalid")
			}
			if err := snapshotdolt.VerifyManaged(binding.Dir, kernel.RepositoryID(binding.ID), allocation); err != nil {
				return nil, err
			}
			repo, err := knowledgedolt.OpenManaged(binding.Dir, kernel.RepositoryID(binding.ID), allocation)
			if err != nil {
				return nil, err
			}
			raw, err := snapshotdolt.OpenManaged(binding.Dir, kernel.RepositoryID(binding.ID), allocation)
			if err != nil {
				_ = repo.Close()
				return nil, err
			}
			return mountDoltFiles(repo, raw), nil
		},
		openExisting: func(abs string, item HomeRepo) (snapshot.Store, error) {
			repo, err := knowledgedolt.OpenExisting(abs, kernel.RepositoryID(item.ID))
			if errors.Is(err, knowledgedolt.ErrNotNativeKnowledge) {
				return snapshotdolt.OpenExisting(abs, kernel.RepositoryID(item.ID))
			}
			if err != nil {
				return nil, err
			}
			raw, err := snapshotdolt.OpenExisting(abs, kernel.RepositoryID(item.ID))
			if err != nil {
				_ = repo.Close()
				return nil, err
			}
			return mountDoltFiles(repo, raw), nil
		},
		open: func(abs string, item HomeRepo) (snapshot.Store, error) {
			repo, err := knowledgedolt.Open(abs, kernel.RepositoryID(item.ID))
			if err != nil {
				return nil, err
			}
			raw, err := snapshotdolt.OpenExisting(abs, kernel.RepositoryID(item.ID))
			if err != nil {
				_ = repo.Close()
				return nil, err
			}
			return mountDoltFiles(repo, raw), nil
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
	"lakefs": {
		connectionOpen: func(binding RepositoryBinding, credential, expected string) (snapshot.Store, string, error) {
			if strings.TrimSpace(credential) == "" {
				return nil, "", kernel.Fail(kernel.ErrUnauthenticated, "repository connection requires its own credential")
			}
			endpoint, err := lakefs.ParseDSN(binding.DSN)
			if err != nil {
				return nil, "", err
			}
			if expected != "" && expected != endpoint.Repository {
				return nil, "", kernel.Fail(kernel.ErrPreconditionFailed, "connected LakeFS authority identity changed")
			}
			repo, err := lakefs.OpenExisting(kernel.RepositoryID(binding.ID), binding.DSN, credential)
			return repo, endpoint.Repository, err
		},
		openExisting: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return lakefs.OpenExisting(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(lakefs.EnvCredential))
		},
		open: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return lakefs.OpenExisting(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(lakefs.EnvCredential))
		},
		discover: func(home, abs string) (HomeRepo, bool) {
			id, dsn, err := lakefs.ReadStamp(abs)
			if err != nil || id == "" {
				return HomeRepo{}, false
			}
			return HomeRepo{ID: id, Dir: homeRel(home, abs), Driver: "lakefs", DSN: dsn}, true
		},
		validate: func(item HomeRepo) error {
			if strings.TrimSpace(item.DSN) == "" {
				return fmt.Errorf("lakefs repository %s is missing dsn", item.ID)
			}
			_, err := lakefs.ParseDSN(item.DSN)
			return err
		},
		stamp: func(abs string, item HomeRepo) error {
			return lakefs.WriteStamp(abs, item.ID, item.DSN)
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
			if _, err := lakefs.ParseDSN(dsn); err != nil {
				return HomeRepo{}, err
			}
			return HomeRepo{ID: spec.ID, Dir: repoDir(stores, spec.ID), Driver: "lakefs", DSN: dsn}, nil
		},
		configure: func(file *StoresFile, _ storeEndpoint) error {
			file.Repository = "lakefs"
			return nil
		},
		secretEnv: lakefs.EnvCredential,
		managedValidate: func(pool ManagedRepositoryConfig, _ DeploymentConfig) error {
			if strings.TrimSpace(pool.DSN) == "" || strings.ContainsAny(pool.DSN, "?#") {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an origin dsn")
			}
			if _, err := lakefs.ParseDSN(strings.TrimRight(pool.DSN, "/") + "/name-probe"); err != nil {
				return err
			}
			namespace, err := url.Parse(strings.TrimSpace(pool.Root))
			if err != nil || namespace.Scheme != "s3" || namespace.Host == "" || namespace.User != nil || namespace.RawQuery != "" || namespace.Fragment != "" {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an s3:// storage namespace prefix in root")
			}
			return nil
		},
		managedBinding: func(pool ManagedRepositoryConfig, req ManagedRepositoryRequest, allocation string) (RepositoryBinding, error) {
			name, err := lakefs.ManagedGravelerName(req.Name, req.Principal, allocation)
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
			if ep, err := lakefs.ParseDSN(binding.DSN); err == nil && ep.Repository != "" {
				name = ep.Repository
			}
			return strings.TrimRight(pool.PublicURL, "/") + "/repositories/" + url.PathEscape(name)
		},
		managedCreate: func(pool ManagedRepositoryConfig, binding RepositoryBinding, allocation string, _ bool) (snapshot.Store, string, error) {
			return lakefs.CreateManaged(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(lakefs.EnvCredential), allocation, pool.Root)
		},
		managedOpen: func(binding RepositoryBinding, allocation, backend string) (snapshot.Store, error) {
			return lakefs.OpenManaged(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(lakefs.EnvCredential), allocation, backend)
		},
	},
	"gitea": {
		managedRestore: func(record managedRecord) snapshot.Store {
			return &managedTreeSource{record: record}
		},
		connectionOpen: func(binding RepositoryBinding, token, expected string) (snapshot.Store, string, error) {
			if strings.TrimSpace(token) == "" {
				return nil, "", kernel.Fail(kernel.ErrUnauthenticated, "repository connection requires its own credential")
			}
			var backend int64
			if expected != "" {
				var err error
				backend, err = strconv.ParseInt(expected, 10, 64)
				if err != nil || backend <= 0 {
					return nil, "", kernel.Fail(kernel.ErrPreconditionFailed, "connection authority receipt is invalid")
				}
			}
			repo, found, err := gitea.OpenConnection(kernel.RepositoryID(binding.ID), binding.DSN, token, backend)
			return repo, strconv.FormatInt(found, 10), err
		},
		managedValidate: func(pool ManagedRepositoryConfig, _ DeploymentConfig) error {
			if pool.Root != "" || strings.TrimSpace(pool.DSN) == "" || strings.ContainsAny(pool.DSN, "?#") {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed Gitea requires an owner URL dsn and does not accept root")
			}
			_, err := gitea.ParseDSN(strings.TrimRight(pool.DSN, "/") + "/kc-probe")
			return err
		},
		managedBinding: func(pool ManagedRepositoryConfig, req ManagedRepositoryRequest, allocation string) (RepositoryBinding, error) {
			base := strings.TrimRight(pool.DSN, "/")
			if validManagedUsername(req.Principal) {
				endpoint, _ := gitea.ParseDSN(base + "/probe")
				base = endpoint.Origin + "/" + url.PathEscape(req.Principal)
			}
			return RepositoryBinding{ID: req.RepositoryID, Driver: "gitea", DSN: base + "/kc-" + allocation}, nil
		},
		managedURL: func(pool ManagedRepositoryConfig, binding RepositoryBinding) string {
			if pool.PublicURL != "" {
				return strings.TrimRight(pool.PublicURL, "/") + "/repositories/" + url.PathEscape(binding.ID)
			}
			return binding.DSN
		},
		managedTrustedOwner: func(req ManagedRepositoryRequest, binding RepositoryBinding) bool {
			ep, err := gitea.ParseDSN(binding.DSN)
			subject, _ := strconv.ParseInt(req.IdentitySubject, 10, 64)
			return err == nil && req.IdentityProvider == "gitea" && subject > 0 && strings.TrimRight(req.IdentityIssuer, "/") == ep.Origin
		},
		managedEnsureOwner: func(record managedRecord) (string, error) {
			ep, err := gitea.ParseDSN(record.Binding.DSN)
			if err != nil {
				return "", err
			}
			trusted := int64(0)
			if record.Request.IdentityProvider == "gitea" && strings.TrimRight(record.Request.IdentityIssuer, "/") == ep.Origin {
				trusted, _ = strconv.ParseInt(record.Request.IdentitySubject, 10, 64)
			}
			known, _ := strconv.ParseInt(record.AccountBackendID, 10, 64)
			backend, err := gitea.EnsureManagedUser(gitea.ManagedUserRequest{Origin: ep.Origin, Username: ep.Owner, EmailDomain: record.AccountEmailDomain, AuthSourceID: record.AccountAuthSourceID, AllocationID: record.AccountAllocationID, BackendID: known, TrustedBackendID: trusted}, os.Getenv(gitea.EnvToken))
			return strconv.FormatInt(backend, 10), err
		},
		managedCreate: func(_ ManagedRepositoryConfig, binding RepositoryBinding, allocation string, userOwned bool) (snapshot.Store, string, error) {
			create := gitea.CreateManaged
			if userOwned {
				create = gitea.CreateManagedForUser
			}
			repo, backend, err := create(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(gitea.EnvToken), allocation)
			return repo, strconv.FormatInt(backend, 10), err
		},
		managedOpen: func(binding RepositoryBinding, allocation, backend string) (snapshot.Store, error) {
			id, err := strconv.ParseInt(backend, 10, 64)
			if err != nil {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed Gitea backend identity is invalid")
			}
			return gitea.OpenManaged(kernel.RepositoryID(binding.ID), binding.DSN, os.Getenv(gitea.EnvToken), allocation, id)
		},
		openExisting: func(_ string, item HomeRepo) (snapshot.Store, error) {
			return gitea.OpenExisting(kernel.RepositoryID(item.ID), item.DSN, os.Getenv(gitea.EnvToken))
		},
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

// openCatalogAuthority opens the independent Catalog Snapshot. Catalog is not a
// Knowledge Repository: Dolt catalogs use the tree adapter only.
func openCatalogAuthority(binding CatalogBinding, create bool) (snapshot.Store, error) {
	switch binding.Driver {
	case "dolt":
		id := kernel.RepositoryID(binding.ID)
		if create {
			return snapshotdolt.OpenDolt(binding.Dir, id)
		}
		return snapshotdolt.OpenExisting(binding.Dir, id)
	case "gitea", "lakefs":
		driver, err := authorityFor(binding.Driver)
		if err != nil {
			return nil, err
		}
		if driver.openExisting == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "catalog authority driver %s cannot open an existing Snapshot", binding.Driver)
		}
		return driver.openExisting("", binding.homeRepo())
	default:
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Catalog %s requires Snapshot driver dolt, gitea or lakefs", binding.ID)
	}
}

// PrepareFixtureAuthority is the explicit acceptance-harness provisioning
// seam. It keeps concrete authority selection in this sole composition root;
// normal deployment startup never calls it and still restores only existing
// authorities.
func PrepareFixtureAuthority(binding RepositoryBinding) error {
	if normalizeRepoDriver(binding.Driver) != "dolt" {
		return nil // remote fixture bindings name authorities provisioned elsewhere
	}
	driver, err := authorityFor(binding.Driver)
	if err != nil {
		return err
	}
	source, err := driver.open(binding.Dir, HomeRepo{
		ID: binding.ID, Driver: binding.Driver, Dir: binding.Dir, DSN: binding.DSN,
	})
	if err != nil {
		return err
	}
	closeManagedSource(source)
	return nil
}
