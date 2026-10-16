package invocation

import (
	"sync"
	"testing"

	"kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type descriptorScopeStore struct {
	snapshot.Store
	objectmodel.Repository
	mu       sync.Mutex
	id       kernel.RepositoryID
	head     kernel.CommitID
	headErr  error
	readErr  error
	heads    int
	reads    map[objectmodel.ObjectID]int
	versions map[kernel.CommitID]map[objectmodel.ObjectID]any
}

func newDescriptorScopeStore(id kernel.RepositoryID, values map[objectmodel.ObjectID]any) *descriptorScopeStore {
	return &descriptorScopeStore{id: id, head: "v1", reads: map[objectmodel.ObjectID]int{}, versions: map[kernel.CommitID]map[objectmodel.ObjectID]any{"v1": values}}
}
func (s *descriptorScopeStore) ID() kernel.RepositoryID { return s.id }
func (*descriptorScopeStore) NativeObjectRepository()   {}
func (*descriptorScopeStore) Archived() bool            { return false }
func (s *descriptorScopeStore) GetRef(string) (kernel.CommitID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.head, s.head != ""
}
func (s *descriptorScopeStore) HasCommit(at kernel.CommitID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.versions[at]
	return found
}
func (s *descriptorScopeStore) Head(ref string) (kernel.CommitID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ref != snapshot.DefaultRef {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "unexpected ref %s", ref)
	}
	s.heads++
	return s.head, s.headErr
}
func (s *descriptorScopeStore) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads[id]++
	if s.readErr != nil {
		return objectmodel.ObjectValue{}, s.readErr
	}
	value, found := s.versions[at][id]
	if !found {
		return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "no descriptor %s at %s", id, at)
	}
	return objectmodel.ObjectValue{Repository: s.id, Commit: at, Value: value}, nil
}
func (s *descriptorScopeStore) advance(at kernel.CommitID, values map[objectmodel.ObjectID]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head, s.versions[at] = at, values
}
func (s *descriptorScopeStore) counts(id objectmodel.ObjectID) (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heads, s.reads[id]
}

type descriptorScopeFixture struct {
	cx      *Invocation
	primary *descriptorScopeStore
	ops     *descriptorScopeStore
}

func newDescriptorScopeFixture(t *testing.T, others ...*descriptorScopeStore) descriptorScopeFixture {
	t.Helper()
	primary := newDescriptorScopeStore("kr://descriptor/main", nil)
	ops := newDescriptorScopeStore("kr://descriptor/ops", descriptorScopeValues("one"))
	registry := snapshot.NewRegistry()
	for _, store := range append([]*descriptorScopeStore{primary, ops}, others...) {
		if err := registry.Add(store); err != nil {
			t.Fatal(err)
		}
	}
	cx := &Invocation{Home: t.TempDir(), Flags: map[string]invoke.FlagValue{"as": "reader"}, WS: &home.Home{Store: registry, Reader: reader.NewReader(registry)}}
	f := descriptorScopeFixture{cx: cx, primary: primary, ops: ops}
	f.allow(t, primary.id, ops.id)
	return f
}
func (f descriptorScopeFixture) allow(t *testing.T, repositories ...kernel.RepositoryID) {
	t.Helper()
	rules := []AllowRule{}
	for _, id := range repositories {
		rules = append(rules, AllowRule{ID: string(id), Principal: "reader", Actions: []string{"object.read"}, Repo: string(id)})
	}
	if err := WriteAllow(f.cx.Home, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
}
func descriptorScopeValues(origin string) map[objectmodel.ObjectID]any {
	return map[objectmodel.ObjectID]any{"ops/stats": origin, "ops/monitor": origin}
}
