package testkit

import (
	"path"
	"sort"
	"strings"
	"sync"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// memoryStore is a private conformance fake, not a selectable authority
// adapter. It deliberately implements only provider-neutral Snapshot ports.
type memoryStore struct {
	mu       sync.RWMutex
	id       kernel.RepositoryID
	refs     map[string]kernel.CommitID
	commits  map[kernel.CommitID]memoryCommit
	archived bool
	sequence int
}

type memoryCommit struct {
	parent kernel.CommitID
	files  map[string][]byte
}

func newMemoryStore(id kernel.RepositoryID) *memoryStore {
	root := kernel.CommitID(kernel.CanonicalDigest(map[string]any{"repository": id, "root": true}))
	return &memoryStore{
		id: id, refs: map[string]kernel.CommitID{snapshot.DefaultRef: root},
		commits: map[kernel.CommitID]memoryCommit{root: {files: map[string][]byte{}}},
	}
}

func (s *memoryStore) ID() kernel.RepositoryID { return s.id }

func (s *memoryStore) Head(ref string) (kernel.CommitID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	commit, ok := s.refs[memoryRef(ref)]
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", ref)
	}
	return commit, nil
}

func (s *memoryStore) GetRef(ref string) (kernel.CommitID, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	commit, ok := s.refs[memoryRef(ref)]
	return commit, ok
}

func (s *memoryStore) HasCommit(commit kernel.CommitID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.commits[commit]
	return ok
}

func (s *memoryStore) CreateRef(ref string, commit kernel.CommitID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.archived {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", s.id)
	}
	if _, ok := s.commits[commit]; !ok {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	ref = memoryRef(ref)
	if _, exists := s.refs[ref]; exists {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	s.refs[ref] = commit
	return nil
}

func (s *memoryStore) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.archived {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", s.id)
	}
	ref = memoryRef(ref)
	if s.refs[ref] != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved", ref)
	}
	if _, ok := s.commits[candidate]; !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", candidate)
	}
	if !s.ancestorLocked(expected, candidate) {
		return "", kernel.Fail(kernel.ErrNonFastForward, "candidate %s is not a descendant of %s", candidate, expected)
	}
	s.refs[ref] = candidate
	return candidate, nil
}

func (s *memoryStore) Archived() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.archived
}

func (s *memoryStore) Archive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archived = true
	return nil
}

func (s *memoryStore) ReadFile(name string, commit kernel.CommitID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.commits[commit]
	if !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	raw, ok := state.files[name]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s does not exist", name)
	}
	return append([]byte(nil), raw...), nil
}

func (s *memoryStore) ListFiles(commit kernel.CommitID) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.commits[commit]
	if !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	out := make([]string, 0, len(state.files))
	for name := range state.files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (s *memoryStore) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.commits[commit]
	if !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	paths := []string{}
	for name, raw := range state.files {
		if !repofile.KnowledgePath(name) {
			continue
		}
		unit := repofile.Parse(string(raw))
		if unit != nil && unit.ObjectID == objectID {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (s *memoryStore) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.archived {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", s.id)
	}
	if change.TargetRepository != s.id {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", change.TargetRepository, s.id)
	}
	ref := memoryRef(change.TargetRef)
	current, exists := s.refs[ref]
	if !exists || current != change.ExpectedTargetCommit || change.BaseCommit != change.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved", ref)
	}
	base, exists := s.commits[change.BaseCommit]
	if !exists {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", change.BaseCommit)
	}
	files := cloneFiles(base.files)
	for _, item := range change.Changes {
		if item.Path == "" || path.IsAbs(item.Path) || strings.HasPrefix(path.Clean(item.Path), "../") {
			return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid tree path %q", item.Path)
		}
		if item.Remove {
			delete(files, item.Path)
		} else {
			files[item.Path] = append([]byte(nil), item.Content...)
		}
	}
	s.sequence++
	commit := kernel.CommitID(kernel.CanonicalDigest(map[string]any{
		"repository": s.id, "parent": current, "sequence": s.sequence, "changes": change.Changes,
	}))
	s.commits[commit] = memoryCommit{parent: current, files: files}
	s.refs[ref] = commit
	return commit, nil
}

func (s *memoryStore) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	files, err := s.ListFiles(request.Commit)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	directory := strings.Trim(strings.TrimSpace(request.Directory), "/")
	prefix := ""
	if directory != "" {
		prefix = directory + "/"
	}
	kinds := map[string]string{}
	for _, name := range files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 1 {
			kinds[parts[0]] = "file"
		} else {
			kinds[parts[0]] = "directory"
		}
	}
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	start := sort.SearchStrings(names, request.Continuation)
	if start < len(names) && names[start] == request.Continuation {
		start++
	}
	limit := request.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	end := start + limit
	if end > len(names) {
		end = len(names)
	}
	page := snapshot.DirectoryPage{Exhausted: end == len(names), Generation: string(request.Commit)}
	for _, name := range names[start:end] {
		page.Entries = append(page.Entries, snapshot.DirectoryEntry{Name: name, Kind: kinds[name]})
	}
	if !page.Exhausted {
		page.Continuation = names[end-1]
	}
	return page, nil
}

func (s *memoryStore) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.commits[commit]; !ok {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = 50
	}
	out := []kernel.CommitID{}
	for current := commit; current != "" && len(out) < limit; current = s.commits[current].parent {
		out = append(out, current)
	}
	return out, nil
}

func (s *memoryStore) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	left, leftOK := s.commits[from]
	right, rightOK := s.commits[to]
	if !leftOK || !rightOK {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "changed-path basis does not exist")
	}
	seen := map[string]struct{}{}
	for name, content := range left.files {
		other, ok := right.files[name]
		if !ok || string(content) != string(other) {
			seen[name] = struct{}{}
		}
	}
	for name := range right.files {
		if _, ok := left.files[name]; !ok {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (s *memoryStore) ancestorLocked(ancestor, commit kernel.CommitID) bool {
	for current := commit; current != ""; current = s.commits[current].parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

func memoryRef(ref string) string {
	if ref == "HEAD" {
		return snapshot.DefaultRef
	}
	return snapshot.RefOrDefault(ref)
}

func cloneFiles(in map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(in))
	for name, raw := range in {
		out[name] = append([]byte(nil), raw...)
	}
	return out
}

var (
	_ snapshot.Store           = (*memoryStore)(nil)
	_ snapshot.TreeStore       = (*memoryStore)(nil)
	_ snapshot.DirectoryReader = (*memoryStore)(nil)
	_ snapshot.HistoryStore    = (*memoryStore)(nil)
	_ snapshot.ChangeStore     = (*memoryStore)(nil)
	_ knowledge.UnitLocator    = (*memoryStore)(nil)
)
