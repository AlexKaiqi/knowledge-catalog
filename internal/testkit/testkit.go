package testkit

import (
	"encoding/base64"
	"fmt"
	"kc/retrieval"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
	"kc/snapshot/commandlog"
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

func MakeRepository(t *testing.T, repositoryID string) *KnowledgeRepository {
	t.Helper()
	if repositoryID == "" {
		repositoryID = "kr://acme/public/core"
	}
	repo := newMemoryStore(kernel.RepositoryID(repositoryID))
	return OpenRepository(t, repo)
}

// KnowledgeRepository is a test-only application assembly. Production
// Snapshot adapters intentionally do not implement knowledge.Repository.
type KnowledgeRepository struct {
	knowledge.Repository
	raw    snapshot.Store
	writer *writer.Writer
	next   int
}

func (*KnowledgeRepository) NativeKnowledgeRepository() {}

func OpenRepository(t *testing.T, raw snapshot.Store) *KnowledgeRepository {
	t.Helper()
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(registry).Require(raw.ID(), kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	return &KnowledgeRepository{Repository: repo, raw: raw, writer: w}
}

func (r *KnowledgeRepository) ApplyKnowledgeCommit(cs knowledge.ChangeSet) (kernel.CommitID, error) {
	r.next++
	receipt, err := r.writer.Commit(fmt.Sprintf("test-fixture-%d", r.next), cs)
	return receipt.Result.CommitID, err
}

func (r *KnowledgeRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	return r.raw.(snapshot.TreeStore).ReadFile(path, commit)
}

func (r *KnowledgeRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	return r.raw.(snapshot.TreeStore).ListFiles(commit)
}

func (r *KnowledgeRepository) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	return r.raw.(snapshot.DirectoryReader).ReadDirectory(request)
}

func (r *KnowledgeRepository) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	locator, ok := r.raw.(knowledge.UnitLocator)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "test repository has no unit locator")
	}
	return locator.ObjectUnitPaths(objectID, commit)
}

func (r *KnowledgeRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	return r.Repository.(knowledge.BatchReadStore).ReadMany(objectIDs, commit)
}

func (r *KnowledgeRepository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	tree, err := testKnowledgeTree(r.raw, commit)
	if err != nil {
		return nil, err
	}
	ids := make([]knowledge.ObjectID, 0)
	for id := range tree.ByObject {
		if knowledge.IsSchemaObject(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *KnowledgeRepository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	tree, err := testKnowledgeTree(r.raw, commit)
	if err != nil {
		return nil, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, unit := range tree.Units {
		if unit.ValueSource == nil || unit.ValueSource.Kind != knowledge.ValueSourceBinding {
			continue
		}
		if parsed, ok := knowledge.ParseSchemaRef(unit.SchemaRef); ok {
			seen[parsed.Object] = struct{}{}
		}
	}
	ids := make([]knowledge.ObjectID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *KnowledgeRepository) SchemaReferrerAddresses(schema knowledge.ObjectID, commit kernel.CommitID) ([]knowledge.Address, error) {
	tree, err := testKnowledgeTree(r.raw, commit)
	if err != nil {
		return nil, err
	}
	addresses := []knowledge.Address{}
	for _, unit := range tree.Units {
		if knowledge.IsSchemaObject(unit.Address.ObjectID) {
			continue
		}
		if parsed, ok := knowledge.ParseSchemaRef(unit.SchemaRef); ok && parsed.Object == schema {
			addresses = append(addresses, unit.Address)
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		return knowledge.AddressKey(addresses[i]) < knowledge.AddressKey(addresses[j])
	})
	return addresses, nil
}

func (r *KnowledgeRepository) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return r.raw.(snapshot.TreeStore).ApplyTreeCommit(cs)
}

func (r *KnowledgeRepository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return r.raw.(snapshot.HistoryStore).CommitHistory(commit, limit)
}

func (r *KnowledgeRepository) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	return r.raw.(snapshot.ChangeStore).ChangedPaths(from, to)
}

// ScanSnapshotPage is deliberately test-only plumbing. Production consumers
// receive knowledge.Repository, whose exact-read contract does not include a
// maintenance scan capability.
func (r *KnowledgeRepository) ScanSnapshotPage(commit kernel.CommitID, request knowledgemaintenance.ScanRequest) (knowledgemaintenance.ScanPage, error) {
	limit, err := knowledgemaintenance.NormalizeScanLimit(request.Limit)
	if err != nil {
		return knowledgemaintenance.ScanPage{}, err
	}
	start := 0
	if request.Continuation != "" {
		raw, decodeErr := base64.RawURLEncoding.DecodeString(request.Continuation)
		parts := strings.Split(string(raw), "\x00")
		if decodeErr != nil || len(parts) != 2 || parts[0] != string(commit) {
			return knowledgemaintenance.ScanPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "test scan continuation does not match commit")
		}
		start, err = strconv.Atoi(parts[1])
		if err != nil || start < 0 {
			return knowledgemaintenance.ScanPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid test scan continuation")
		}
	}
	tree, err := testKnowledgeTree(r.raw, commit)
	if err != nil {
		return knowledgemaintenance.ScanPage{}, err
	}
	ids := make([]knowledge.ObjectID, 0, len(tree.ByObject))
	for id := range tree.ByObject {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	page := knowledgemaintenance.ScanPage{Exhausted: end == len(ids)}
	for _, id := range ids[start:end] {
		units := tree.ObjectUnits(id)
		value, assembleErr := repofile.Assemble(units)
		if assembleErr != nil {
			return knowledgemaintenance.ScanPage{}, assembleErr
		}
		page.Values = append(page.Values, knowledge.KnowledgeValue{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: r.ID(), Object: id}, Repository: r.ID(),
			Commit: commit, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: id},
			Value: value, Declarations: repofile.Declarations(units),
		})
	}
	if !page.Exhausted {
		page.Continuation = base64.RawURLEncoding.EncodeToString([]byte(string(commit) + "\x00" + strconv.Itoa(end)))
	}
	return page, nil
}

func testKnowledgeTree(raw snapshot.Store, commit kernel.CommitID) (*repofile.Tree, error) {
	treeStore, ok := raw.(snapshot.TreeStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "test repository has no tree access")
	}
	paths, err := treeStore.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	tree := repofile.NewTree()
	for _, name := range paths {
		if !repofile.KnowledgePath(name) {
			continue
		}
		raw, err := treeStore.ReadFile(name, commit)
		if err != nil {
			return nil, err
		}
		if unit := repofile.Parse(string(raw)); unit != nil {
			if err := repofile.Ingest(tree, unit, name); err != nil {
				return nil, err
			}
		}
	}
	return tree, nil
}

type Setup struct {
	Repo         *KnowledgeRepository
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

func MustHead(t *testing.T, repo snapshot.Store, ref string) kernel.CommitID {
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
	return reader.Open(reader.Lookup(cat.Require), WorkspacePin(resolved)), nil
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
	return retrieval.PlanAccess(reader.Lookup(cat.Require), WorkspacePin(resolved))
}
