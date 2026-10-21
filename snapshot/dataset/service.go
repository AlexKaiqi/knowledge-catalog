// Package dataset owns Snapshot Store file composition and publication.
// It consumes opaque paths and fixed commits without interpreting Object.
package dataset

import (
	"strings"
	"sync"
	"sync/atomic"

	"kc/kernel"
	"kc/snapshot"
)

// service is combination over a set of Snapshot repositories.
//
//	Dataset — published Dataset: file pointers frozen to commits at define
//	ResolvedDataset  — ResolveDataset returns that version's file list and implied {Repository → commit}
//
// service composes files over snapshot.Store and is not an object
// protocol. Object wrapping lives in object/{writer,reader} and index. Dynamic State/Stream
// observation belongs to an upper-layer Materialization runtime; this package
// freezes only Repository commits.
//
// Operations, by what they change:
//
//	recipe:   DefineDataset, Workspace, RetireDataset
//	resolve:  ResolveDataset / CheckResolved
//	register: RegisterRepository
//	history:  Log
//	hooks:    AddHook / NotifySnapshot (in-process; not outbound kc hook-add)
//
// object_id is not a service concern. Consumer Read lives in object/reader;
// AccessSpec lives in retrieval/.
type service struct {
	*registryState
	// Request context: one service value per request carries its own evidence
	// observer and stamp; the accepted registry state is shared. ctxMu guards
	// only these fields on a value that is stamped after construction.
	readOnly  bool
	ctxMu     sync.Mutex
	observer  Observer
	as        string
	requestID string
	ruleID    string
}

// registryState is the accepted registry shared by every request view of one
// authority. Readers load the current immutable version and never wait:
// a write prepares and validates outside any lock, then commit serializes only
// the read-modify-persist of the single registry document (persist also checks
// the expected registry head) and swaps the accepted version.
type registryState struct {
	writeMu  sync.Mutex
	store    snapshot.Registry
	registry *registry
	current  atomic.Pointer[acceptedState]
	hooks    []Hook
	fence    revisionFence
}

// acceptedState is one accepted registry version. It is never modified after
// it is published; a write builds a successor and swaps the pointer, so a
// reader holding it keeps reading the old version.
type acceptedState struct {
	head         string
	datasets     map[string]Dataset
	versions     map[string]map[int]Dataset
	repositories map[string]struct{}
}

func NewService(store snapshot.Registry, source Registry) (Service, error) {
	registry, ok := source.(*registry)
	if !ok || registry == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "service registry is required")
	}
	c := &service{registryState: &registryState{store: store, registry: registry}}
	state, head, err := registry.loadSnapshot()
	if err != nil {
		return nil, err
	}
	c.current.Store(newAcceptedState(state, head))
	if store != nil {
		store.OnAdvanced(func(ev snapshot.Advanced) {
			c.NotifySnapshot(Snapshot{
				Repository: ev.Store,
				From:       ev.From,
				To:         ev.To,
			})
		})
	}
	return c, nil
}

// accepted is the registry version this value serves: the latest accepted
// version for the shared service and request views, the frozen one for a
// read view.
func (c *service) accepted() *acceptedState { return c.current.Load() }

// DumpState snapshots the registry maps. Not a dump of those Repositories' objects.
func (c *service) DumpState() State {
	return c.accepted().dump(c.registry.AuthorityID())
}

func (s *acceptedState) dump(authorityID string) State {
	workspaces := make([]Dataset, 0, len(s.datasets))
	for _, workspace := range s.datasets {
		workspaces = append(workspaces, cloneDataset(workspace))
	}
	return State{
		Datasets:        workspaces,
		DatasetVersions: s.datasetVersions(),
		Repositories:    s.repositoriesList(),
		AuthorityID:     authorityID,
	}
}

// LoadState replaces the in-memory version from registry bytes without
// persisting it; the accepted registry head is kept.
func (c *service) LoadState(state State) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.current.Store(newAcceptedState(state, c.accepted().head))
}

func newAcceptedState(state State, head string) *acceptedState {
	s := &acceptedState{
		head:         head,
		datasets:     map[string]Dataset{},
		versions:     map[string]map[int]Dataset{},
		repositories: map[string]struct{}{},
	}
	for _, id := range state.Repositories {
		s.repositories[id] = struct{}{}
	}
	for _, workspace := range state.Datasets {
		s.datasets[workspace.SetID] = cloneDataset(workspace)
		// Existing registries have only the current release. Preserve that
		// accepted release on the first write; never invent older versions.
		s.loadVersion(workspace)
	}
	for _, version := range state.DatasetVersions {
		s.loadVersion(version)
	}
	return s
}

func (s *acceptedState) loadVersion(def Dataset) {
	if s.versions[def.SetID] == nil {
		s.versions[def.SetID] = map[int]Dataset{}
	}
	def.Retired = false // retirement is current policy, not versioned content
	s.versions[def.SetID][def.Revision] = cloneDataset(def)
}

func (s *acceptedState) datasetVersions() []Dataset {
	var out []Dataset
	for _, versions := range s.versions {
		for _, def := range versions {
			out = append(out, cloneDataset(def))
		}
	}
	return normalizeDatasetVersions(out)
}

// commit is the only mutual exclusion a registry write takes. mutate derives
// the next state from the latest accepted version and must stay in memory:
// remote reads and validation belong before commit, so neither readers nor
// other writers wait on them. The registry head CAS still rejects a version
// another process advanced.
func (c *service) commit(message string, mutate func(*acceptedState) (State, error)) error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a service read view cannot persist changes")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	current := c.accepted()
	next, err := mutate(current)
	if err != nil {
		return err
	}
	as, requestID, ruleID, observer := c.requestContext()
	head, err := c.registry.saveExpected(current.head, next, message, as, requestID, ruleID)
	if err == nil {
		c.current.Store(newAcceptedState(next, head))
	}
	cmd, _, _ := strings.Cut(strings.TrimSpace(message), " ")
	if cmd == "" {
		cmd = "persist"
	}
	return observe(observer, cmd, map[string]any{
		"authorityId": c.registry.AuthorityID(),
		"message":     message,
	}, err)
}

func (c *service) requestContext() (string, string, string, Observer) {
	c.ctxMu.Lock()
	defer c.ctxMu.Unlock()
	return c.as, c.requestID, c.ruleID, c.observer
}

func (c *service) SetObserver(j Observer) { c.ctxMu.Lock(); defer c.ctxMu.Unlock(); c.observer = j }

func (c *service) SetStamp(as, requestID, ruleID string) {
	c.ctxMu.Lock()
	defer c.ctxMu.Unlock()
	c.as = as
	c.requestID = requestID
	c.ruleID = ruleID
}

func (c *service) note(cmd string, refs map[string]any, err error) error {
	if refs == nil {
		refs = map[string]any{}
	}
	if _, ok := refs["authorityId"]; !ok && c.registry != nil {
		refs["authorityId"] = c.registry.AuthorityID()
	}
	_, _, _, observer := c.requestContext()
	return observe(observer, cmd, refs, err)
}

// RecordCreated writes registry.yaml so service.Log has a birth commit (empty registry is otherwise only git "root").
func (c *service) RecordCreated() error {
	return c.commit("init "+c.registry.AuthorityID(), func(s *acceptedState) (State, error) {
		return s.dump(c.registry.AuthorityID()), nil
	})
}
