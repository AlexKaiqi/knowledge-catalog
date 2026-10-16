package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// Neither an unreadable local Schema nor an unavailable possible remote
// holder proves absence. Both check and publication must preserve the failure.
func TestIndexDeclarationSchemaResolutionPreservesReadFailures(t *testing.T) {
	for _, failure := range []string{"local Schema body", "remote source Head"} {
		for _, entrypoint := range []string{"check", "commit"} {
			t.Run(failure+"/"+entrypoint, func(t *testing.T) {
				s := testkit.NewSetup(t, "kr://index-resolution/consumer")
				before := s.RootCommitID
				if failure == "local Schema body" {
					before = commitIndexWriterDrafts(t, s, s.Repo, "local-schema",
						indexWriterSchema("schema/article", "definition", "body"))
				}
				for _, name := range []string{"kr://index-resolution/source-a", "kr://index-resolution/source-b"} {
					if failure == "local Schema body" && name == "kr://index-resolution/source-b" {
						continue
					}
					repo := testkit.MakeRepository(t, name)
					if err := s.Store.Add(repo); err != nil {
						t.Fatal(err)
					}
					commitIndexWriterDrafts(t, s, repo, "source-schema-"+name,
						indexWriterSchema("schema/article", "definition", "body"))
				}
				s.Store.SetWrap(func(store snapshot.Store) snapshot.Store {
					if failure == "local Schema body" && store.ID() == s.RepositoryID {
						return indexDeclarationResolutionFaultStore{Store: store, TreeStore: store.(snapshot.TreeStore),
							failedPath: repofile.DefaultSchemaPath("schema/article")}
					}
					if failure == "remote source Head" && store.ID() == "kr://index-resolution/source-b" {
						return indexDeclarationResolutionFaultStore{Store: store, TreeStore: store.(snapshot.TreeStore), failHead: true}
					}
					return store
				})
				declaration := testkit.IndexText("schema/article", "body")
				cs := objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
					BaseCommit: before, ExpectedTargetCommit: before, Operations: []objectmodel.Operation{declaration}}
				var err error
				if entrypoint == "check" {
					_, _, err = s.Writer.CheckIndexDeclaration(cs, declaration.Address)
				} else {
					_, err = s.Writer.Commit("source-resolution-failure", cs)
				}
				if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
					t.Errorf("%s must preserve the Schema source failure, got %v", entrypoint, err)
				}
				if head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); head != before {
					t.Errorf("failed source resolution advanced the target: before=%s after=%s", before, head)
				}
			})
		}
	}
}

type indexDeclarationResolutionFaultStore struct {
	snapshot.Store
	snapshot.TreeStore
	failedPath string
	failHead   bool
}

func (s indexDeclarationResolutionFaultStore) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if path == s.failedPath {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "Schema body is temporarily unavailable")
	}
	return s.TreeStore.ReadFile(path, commit)
}

func (s indexDeclarationResolutionFaultStore) Head(ref string) (kernel.CommitID, error) {
	if s.failHead {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "Schema source Head is temporarily unavailable")
	}
	return s.Store.Head(ref)
}
