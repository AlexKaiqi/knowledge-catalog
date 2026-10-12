package testkit

import (
	"testing"

	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// MakeSystemRepository publishes onto an actual Snapshot fixture and uses the
// same tree interpretation path as deployed system repositories.
func MakeSystemRepository(t *testing.T) *KnowledgeRepository {
	t.Helper()
	source := MakeTreeStore(t, string(knowledge.SystemRepositoryID))
	if _, err := writer.PublishSystem(source); err != nil {
		t.Fatal(err)
	}
	return OpenRepository(t, source)
}

// nativeSystemRepository deliberately exposes native reads and no raw write
// port, for provider dispatch and capability preservation contracts.
type nativeSystemRepository struct {
	knowledge.Repository
	snapshot.TreeReader
	snapshot.DirectoryReader
}

func (*nativeSystemRepository) NativeKnowledgeRepository() {}

func MakeNativeSystemRepository(t *testing.T) knowledge.NativeRepository {
	t.Helper()
	repo := MakeSystemRepository(t)
	return &nativeSystemRepository{repo, repo, repo}
}
