package testkit

import (
	"kc/retrieval"
	"os"
	"path/filepath"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
	"kc/snapshot/commandlog"
	"kc/snapshot/filegit"
	"kc/snapshot/treewriter"
)

func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func MakeRepository(t *testing.T, repositoryID string) *filegit.FileGitRepository {
	t.Helper()
	if repositoryID == "" {
		repositoryID = "kr://acme/public/core"
	}
	repo, err := filegit.NewFileGit(TempDir(t), kernel.RepositoryID(repositoryID))
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

type Setup struct {
	Repo         *filegit.FileGitRepository
	Store        *snapshot.Registry
	Writer       *writer.Writer
	TreeWriter   *treewriter.Writer
	Reader       *reader.Reader
	RepositoryID kernel.RepositoryID
	RootCommitID kernel.CommitID
}

func NewSetup(t *testing.T, repositoryID string) Setup {
	t.Helper()
	if repositoryID == "" {
		repositoryID = "kr://acme/public/core"
	}
	repo := MakeRepository(t, repositoryID)
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	commands, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, commands)
	if err != nil {
		t.Fatal(err)
	}
	tw, err := treewriter.New(store, commands)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	return Setup{
		Repo:         repo,
		Store:        store,
		Writer:       w,
		TreeWriter:   tw,
		Reader:       reader.NewReader(store),
		RepositoryID: kernel.RepositoryID(repositoryID),
		RootCommitID: head,
	}
}

func MakeRegistry(t *testing.T) *catalog.Registry {
	t.Helper()
	registry, err := catalog.NewRegistry(filepath.Join(TempDir(t), "catalog"), "kr://acme/catalog")
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func OpenCatalog(t *testing.T, store *snapshot.Registry) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewCatalog(store, MakeRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	RegisterMounted(t, cat, store)
	return cat
}

func RegisterMounted(t *testing.T, cat *catalog.Catalog, store *snapshot.Registry) {
	t.Helper()
	for _, id := range store.IDs() {
		if err := cat.RegisterRepository(id); err != nil {
			t.Fatal(err)
		}
	}
}

func MustHead(t *testing.T, repo knowledge.Repository, ref string) kernel.CommitID {
	t.Helper()
	head, err := repo.Head(ref)
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func ExpectCode(t *testing.T, err error, code kernel.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s but nothing was returned", code)
	}
	if got := kernel.CodeOf(err); got != code {
		t.Fatalf("expected error code %s, got %s (%v)", code, got, err)
	}
}

func PutEntity(objectID string, value any, pathHint string) []knowledge.Operation {
	op := knowledge.Operation{
		Op:      knowledge.OpPut,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(objectID)},
		Value:   value,
	}
	if pathHint != "" {
		op.PathHint = pathHint
	}
	return []knowledge.Operation{op}
}

func CommitChange(repoID kernel.RepositoryID, base kernel.CommitID, objectID string, value any, pathHint string) knowledge.ChangeSet {
	return knowledge.ChangeSet{
		TargetRepository:     repoID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           base,
		ExpectedTargetCommit: base,
		Operations:           PutEntity(objectID, value, pathHint),
	}
}

func MustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func WorkspacePin(resolved catalog.ResolvedWorkspace) reader.WorkspacePin {
	return reader.WorkspacePin{
		WorkspaceID:  resolved.WorkspaceID,
		Revision:     resolved.Revision,
		Repositories: resolved.Repositories,
	}
}

func OpenWorkspace(cat *catalog.Catalog, workspaceID string) (*reader.Serving, error) {
	resolved, err := cat.ResolveWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return reader.Open(knowledge.Lookup(cat.Require), WorkspacePin(resolved)), nil
}

func FederatedRead(cat *catalog.Catalog, workspaceID string, objectID knowledge.ObjectID) ([]reader.FederatedValue, error) {
	serving, err := OpenWorkspace(cat, workspaceID)
	if err != nil {
		return nil, err
	}
	return serving.Read(objectID, nil)
}

func PlanAccess(cat *catalog.Catalog, workspaceID string) (retrieval.AccessPlan, error) {
	resolved, err := cat.ResolveWorkspace(workspaceID)
	if err != nil {
		return retrieval.AccessPlan{}, err
	}
	return retrieval.PlanAccess(knowledge.Lookup(cat.Require), WorkspacePin(resolved))
}
