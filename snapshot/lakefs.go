package snapshot

import (
	"context"

	"kc/kernel"
	"kc/snapshot/internal/lakefsimpl"
	"kc/snapshot/internal/stamp"
)

// LakeFSCredentialEnv names the private credential environment variable.
const LakeFSCredentialEnv = lakefsimpl.EnvCredential

// LakeFSEndpoint is the public, non-secret location parsed from a lakeFS DSN.
type LakeFSEndpoint struct {
	Origin     string
	API        string
	Repository string
}

var (
	_ Store                = (*lakefsimpl.Repository)(nil)
	_ StoreOrigin          = (*lakefsimpl.Repository)(nil)
	_ TreeStore            = (*lakefsimpl.Repository)(nil)
	_ DirectoryReader      = (*lakefsimpl.Repository)(nil)
	_ HistoryStore         = (*lakefsimpl.Repository)(nil)
	_ ChangeStore          = (*lakefsimpl.Repository)(nil)
	_ BulkTreeIngester     = (*lakefsimpl.Repository)(nil)
	_ CommitMetadataReader = (*lakefsimpl.Repository)(nil)
)

// ContextBoundStore lets a Store wrapper forward cancellation without exposing
// its inner authority. The returned view must preserve identity, optional
// capabilities and restrictions, and must not mutate the original Store.
type ContextBoundStore interface {
	WithContext(context.Context) Store
}

// BindStoreContext returns a view of store whose network reads are canceled
// with ctx. Wrappers forward the binding through ContextBoundStore. Stores
// without this capability, and a nil ctx, come back unchanged; absence of the
// capability does not imply that a Store performs no network I/O.
func BindStoreContext(store Store, ctx context.Context) Store {
	if ctx == nil {
		return store
	}
	if wrapper, ok := store.(ContextBoundStore); ok {
		return wrapper.WithContext(ctx)
	}
	if repo, ok := store.(*lakefsimpl.Repository); ok {
		return repo.WithContext(ctx)
	}
	return store
}

// OpenLakeFS attaches an existing lakeFS authority. Optional file capabilities
// remain discoverable through the root capability interfaces.
func OpenLakeFS(id kernel.RepositoryID, dsn, credential string) (Store, error) {
	return OpenLakeFSContext(context.Background(), id, dsn, credential)
}

// OpenLakeFSContext also binds the authority's opening requests to ctx, so
// cancellation applies before a Store view is available.
func OpenLakeFSContext(ctx context.Context, id kernel.RepositoryID, dsn, credential string) (Store, error) {
	repo, err := lakefsimpl.OpenExistingContext(ctx, id, dsn, credential)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

// RecoverLakeFSPublication releases an interrupted publication's coordination
// branch after all writers to this authority have been stopped. It compares
// both observed coordinates, never advances the target, and is idempotent
// when the lock is already absent and the target still matches. This trusted
// operator operation does not resolve a pending command receipt. Stock lakeFS
// has no fencing token; callers must keep publishers stopped through recovery.
func RecoverLakeFSPublication(store Store, targetRef string, expectedTarget, expectedCandidate kernel.CommitID) error {
	repo, ok := store.(*lakefsimpl.Repository)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "publication recovery requires a direct lakeFS authority")
	}
	return repo.RecoverPublication(targetRef, expectedTarget, expectedCandidate)
}

// CreateManagedLakeFS creates or resumes the named managed allocation and
// returns its durable storage namespace alongside the Store contract.
func CreateManagedLakeFS(id kernel.RepositoryID, dsn, credential, allocation, storagePrefix string) (Store, string, error) {
	repo, backend, err := lakefsimpl.CreateManaged(id, dsn, credential, allocation, storagePrefix)
	if err != nil {
		return nil, "", err
	}
	return repo, backend, nil
}

// OpenManagedLakeFS reopens a managed allocation, rejecting a missing or
// replaced repository instead of recreating it.
func OpenManagedLakeFS(id kernel.RepositoryID, dsn, credential, allocation, backendID string) (Store, error) {
	repo, err := lakefsimpl.OpenManaged(id, dsn, credential, allocation, backendID)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func ParseLakeFSDSN(raw string) (LakeFSEndpoint, error) {
	ep, err := lakefsimpl.ParseDSN(raw)
	if err != nil {
		return LakeFSEndpoint{}, err
	}
	return LakeFSEndpoint{Origin: ep.Origin, API: ep.API, Repository: ep.Repository}, nil
}

func LakeFSSlug(raw string) string { return lakefsimpl.GravelerSlug(raw) }

func ManagedLakeFSName(name, owner, allocation string) (string, error) {
	return lakefsimpl.ManagedGravelerName(name, owner, allocation)
}

func PlatformLakeFSName(name string) bool { return lakefsimpl.PlatformGravelerName(name) }

func ValidLakeFSName(name string) bool { return lakefsimpl.ValidGravelerName(name) }

// WriteLakeFSStamp stores a public reopening pointer without credentials.
func WriteLakeFSStamp(dir, repositoryID, dsn string) error {
	return stamp.Write(dir, "lakefs", repositoryID, dsn)
}

func ReadLakeFSStamp(dir string) (id, dsn string, err error) {
	return stamp.Read(dir, "lakefs")
}
