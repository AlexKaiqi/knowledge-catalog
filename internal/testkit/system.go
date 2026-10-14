package testkit

import (
	"kc/systemrepo"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// MakeSystemRepository publishes onto an actual Snapshot fixture and uses the
// same tree interpretation path as deployed system repositories.
func MakeSystemRepository(t *testing.T) *KnowledgeRepository {
	t.Helper()
	source := MakeTreeStore(t, string(objectmodel.SystemRepositoryID))
	if _, err := systemrepo.PublishSystem(source); err != nil {
		t.Fatal(err)
	}
	return OpenRepository(t, source)
}

// nativeSystemRepository deliberately exposes native reads and no raw write
// port, for provider dispatch and capability preservation contracts.
type nativeSystemRepository struct {
	objectmodel.Repository
	snapshot.TreeReader
	snapshot.DirectoryReader
	control snapshot.Store
}

func (r *nativeSystemRepository) CreateRef(ref string, at kernel.CommitID) error {
	return r.control.CreateRef(ref, at)
}
func (r *nativeSystemRepository) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return r.control.Merge(ref, candidate, expected)
}
func (r *nativeSystemRepository) Archive() error { return r.control.Archive() }

func (*nativeSystemRepository) NativeKnowledgeRepository() {}

type NativeSnapshotRepository interface {
	objectmodel.NativeRepository
	snapshot.Store
}

func MakeNativeSystemRepository(t *testing.T) NativeSnapshotRepository {
	t.Helper()
	repo := MakeSystemRepository(t)
	return &nativeSystemRepository{repo, repo, repo, repo}
}
