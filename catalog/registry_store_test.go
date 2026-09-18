package catalog_test

import (
	"sync"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

type catalogMem struct {
	mu      sync.Mutex
	id      kernel.RepositoryID
	refs    map[string]kernel.CommitID
	commits map[kernel.CommitID]map[string][]byte
	seq     int
}

func newCatalogMem(id string) *catalogMem {
	root := kernel.CommitID(kernel.CanonicalDigest(map[string]any{"catalog": id, "root": true}))
	return &catalogMem{
		id:      kernel.RepositoryID(id),
		refs:    map[string]kernel.CommitID{snapshot.DefaultRef: root},
		commits: map[kernel.CommitID]map[string][]byte{root: {}},
	}
}

func (s *catalogMem) ID() kernel.RepositoryID { return s.id }
func (s *catalogMem) Head(ref string) (kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	commit, ok := s.refs[snapshot.RefOrDefault(ref)]
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref missing")
	}
	return commit, nil
}
func (s *catalogMem) GetRef(ref string) (kernel.CommitID, bool) {
	c, err := s.Head(ref)
	return c, err == nil
}
func (s *catalogMem) HasCommit(commit kernel.CommitID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.commits[commit]
	return ok
}
func (s *catalogMem) CreateRef(string, kernel.CommitID) error { return nil }
func (s *catalogMem) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrUsageInvalid, "unused")
}
func (s *catalogMem) Archived() bool { return false }
func (s *catalogMem) Archive() error { return nil }
func (s *catalogMem) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, ok := s.commits[commit]
	if !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	body, ok := files[path]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing %s", path)
	}
	return append([]byte(nil), body...), nil
}
func (s *catalogMem) ListFiles(commit kernel.CommitID) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, ok := s.commits[commit]
	if !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	out := make([]string, 0, len(files))
	for path := range files {
		out = append(out, path)
	}
	return out, nil
}
func (s *catalogMem) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := snapshot.RefOrDefault(cs.TargetRef)
	current := s.refs[ref]
	if current != cs.ExpectedTargetCommit || cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "catalog authority moved")
	}
	next := map[string][]byte{}
	for path, body := range s.commits[current] {
		next[path] = append([]byte(nil), body...)
	}
	for _, change := range cs.Changes {
		if change.Remove {
			delete(next, change.Path)
			continue
		}
		next[change.Path] = append([]byte(nil), change.Content...)
	}
	s.seq++
	commit := kernel.CommitID(kernel.CanonicalDigest(map[string]any{"n": s.seq, "files": next}))
	s.commits[commit] = next
	s.refs[ref] = commit
	return commit, nil
}

func TestSnapshotRegistryPersistsMembershipWithoutKnowledgeSemantics(t *testing.T) {
	id := "kr://acme/catalog"
	store := newCatalogMem(id)
	created, err := catalog.CreateSnapshotRegistry(store, id, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(snapshot.NewRegistry(), created)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("kr://acme/core"); err != nil {
		t.Fatal(err)
	}
	head, err := created.Head()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := catalog.OpenSnapshotRegistry(store, id, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Head()
	if err != nil || got != head {
		t.Fatalf("reopen changed Catalog head: %s -> %s %v", head, got, err)
	}
	state, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.CatalogID != id || len(state.Repositories) != 1 || state.Repositories[0] != "kr://acme/core" {
		t.Fatalf("registry lost membership: %#v", state)
	}
}

func TestSnapshotRegistryRejectsStaleHandle(t *testing.T) {
	id := "kr://acme/catalog"
	store := newCatalogMem(id)
	first, err := catalog.CreateSnapshotRegistry(store, id, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalog.OpenSnapshotRegistry(store, id, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(snapshot.NewRegistry(), second)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("kr://acme/core"); err != nil {
		t.Fatal(err)
	}
	stale, err := catalog.NewCatalog(snapshot.NewRegistry(), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.RegisterRepository("kr://acme/other"); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale Catalog handle overwrote newer membership: %v", err)
	}
}
