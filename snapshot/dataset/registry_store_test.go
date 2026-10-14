package dataset_test

import (
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

type catalogMem struct {
	mu       sync.Mutex
	id       kernel.RepositoryID
	refs     map[string]kernel.CommitID
	commits  map[kernel.CommitID]map[string][]byte
	parents  map[kernel.CommitID]kernel.CommitID
	metadata map[kernel.CommitID]snapshot.CommitMetadata
	applyErr error
	seq      int
}

func newCatalogMem(id string) *catalogMem {
	root := kernel.CommitID(kernel.CanonicalDigest(map[string]any{"catalog": id, "root": true}))
	return &catalogMem{
		id:       kernel.RepositoryID(id),
		refs:     map[string]kernel.CommitID{snapshot.DefaultRef: root},
		commits:  map[kernel.CommitID]map[string][]byte{root: {}},
		parents:  map[kernel.CommitID]kernel.CommitID{},
		metadata: map[kernel.CommitID]snapshot.CommitMetadata{root: {Commit: root}},
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
func (s *catalogMem) CreateRef(ref string, commit kernel.CommitID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.commits[commit]; !ok {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	ref = snapshot.RefOrDefault(ref)
	if _, ok := s.refs[ref]; ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref already exists")
	}
	s.refs[ref] = commit
	return nil
}
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
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "missing %s", path)
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
	if s.applyErr != nil {
		return "", s.applyErr
	}
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
	s.parents[commit] = current
	s.metadata[commit] = snapshot.CommitMetadata{Commit: commit, Author: cs.Author, Message: cs.Message, RequestID: cs.RequestID, RuleID: cs.RuleID}
	s.refs[ref] = commit
	return commit, nil
}

func (s *catalogMem) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.commits[commit]; !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	var out []kernel.CommitID
	for commit != "" && (limit <= 0 || len(out) < limit) {
		out = append(out, commit)
		commit = s.parents[commit]
	}
	return out, nil
}

func TestSnapshotRegistryPersistsMembershipWithoutObjectSemantics(t *testing.T) {
	id := "kr://acme/catalog"
	store := newCatalogMem(id)
	created, err := catalog.CreateSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(snapshot.NewRegistry(), created)
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
	reopened, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Head()
	if err != nil || got != head {
		t.Fatalf("reopen changed Service head: %s -> %s %v", head, got, err)
	}
	state, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AuthorityID != id || len(state.Repositories) != 1 || state.Repositories[0] != "kr://acme/core" {
		t.Fatalf("registry lost membership: %#v", state)
	}
}

func TestSnapshotRegistryRejectsStaleHandle(t *testing.T) {
	id := "kr://acme/catalog"
	store := newCatalogMem(id)
	first, err := catalog.CreateSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(snapshot.NewRegistry(), second)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("kr://acme/core"); err != nil {
		t.Fatal(err)
	}
	stale, err := catalog.NewService(snapshot.NewRegistry(), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.RegisterRepository("kr://acme/other"); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale Service handle overwrote newer membership: %v", err)
	}
}

func (s *catalogMem) ReadCommitMetadata(commit kernel.CommitID) (snapshot.CommitMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, ok := s.metadata[commit]
	if !ok {
		return snapshot.CommitMetadata{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	return metadata, nil
}

func createCatalogRegistry(t *testing.T, id string) (*catalogMem, catalog.Registry) {
	t.Helper()
	authority := newCatalogMem(id)
	registry, err := catalog.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	return authority, registry
}
