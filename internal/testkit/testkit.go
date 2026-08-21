package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
	"kc/writer"
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

func MakeRepository(t *testing.T, repositoryID string) *local.FileGitRepository {
	t.Helper()
	if repositoryID == "" {
		repositoryID = "kr://acme/public/core"
	}
	repo, err := local.NewFileGit(TempDir(t), kernel.RepositoryID(repositoryID))
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

type Setup struct {
	Repo         *local.FileGitRepository
	Stream       *local.JSONLStream
	Store        *repository.Store
	Writer       *writer.Writer
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
	store := repository.NewStore()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	BindJSONL(t, store, repo)
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head("")
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := store.GetStream(repo.ID())
	return Setup{
		Repo:         repo,
		Stream:       stream.(*local.JSONLStream),
		Store:        store,
		Writer:       w,
		Reader:       reader.NewReader(store),
		RepositoryID: kernel.RepositoryID(repositoryID),
		RootCommitID: head,
	}
}

func BindJSONL(t *testing.T, store *repository.Store, repo repository.Repository) {
	t.Helper()
	rooted, ok := repo.(interface{ RootDir() string })
	if !ok {
		return
	}
	if err := store.AddStream(repo.ID(), local.NewJSONLStream(rooted.RootDir(), repo.ID())); err != nil {
		t.Fatal(err)
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

func OpenCatalog(t *testing.T, store *repository.Store) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewCatalog(store, MakeRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	RegisterMounted(t, cat, store)
	return cat
}

func RegisterMounted(t *testing.T, cat *catalog.Catalog, store *repository.Store) {
	t.Helper()
	for _, id := range store.IDs() {
		if err := cat.RegisterRepository(id); err != nil {
			t.Fatal(err)
		}
	}
}

func MustHead(t *testing.T, repo repository.Repository, ref string) kernel.CommitID {
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

func PutEntity(objectID string, value any, pathHint string) []repository.Operation {
	op := repository.Operation{
		Op:      repository.OpPut,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: kernel.ObjectID(objectID)},
		Value:   value,
	}
	if pathHint != "" {
		op.PathHint = pathHint
	}
	return []repository.Operation{op}
}

func CommitChange(repoID kernel.RepositoryID, base kernel.CommitID, objectID string, value any, pathHint string) repository.CommitChangeSet {
	return repository.CommitChangeSet{
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
