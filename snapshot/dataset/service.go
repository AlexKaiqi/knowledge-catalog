// Package dataset owns Snapshot Store file composition and publication.
// It consumes opaque paths and fixed commits without interpreting Object.
package dataset

import (
	"strings"
	"sync"

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
	mu           sync.RWMutex
	store        snapshot.Registry
	registry     *registry
	registryHead string
	datasets     map[string]Dataset
	versions     map[string]map[int]Dataset
	repositories map[string]struct{}
	readOnly     bool
	observer     Observer
	as           string
	requestID    string
	ruleID       string
	hooks        []Hook
}

func NewService(store snapshot.Registry, source Registry) (Service, error) {
	registry, ok := source.(*registry)
	if !ok || registry == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "service registry is required")
	}
	c := &service{
		store:        store,
		registry:     registry,
		datasets:     map[string]Dataset{},
		repositories: map[string]struct{}{},
	}
	state, head, err := registry.loadSnapshot()
	if err != nil {
		return nil, err
	}
	c.LoadState(state)
	c.registryHead = head
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

// DumpState snapshots the registry maps. Not a dump of those Repositories' objects.
func (c *service) DumpState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dumpState()
}

func (c *service) dumpState() State {
	workspaces := make([]Dataset, 0, len(c.datasets))
	for _, workspace := range c.datasets {
		workspaces = append(workspaces, cloneDataset(workspace))
	}
	return State{
		Datasets:        workspaces,
		DatasetVersions: c.datasetVersions(),
		Repositories:    c.repositoriesList(),
		AuthorityID:     c.registry.AuthorityID(),
	}
}

// LoadState replaces the maps from registry bytes. Called at construct.
func (c *service) LoadState(state State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadState(state)
}

func (c *service) loadState(state State) {
	c.datasets = map[string]Dataset{}
	c.versions = map[string]map[int]Dataset{}
	c.repositories = map[string]struct{}{}
	for _, id := range state.Repositories {
		c.repositories[id] = struct{}{}
	}
	for _, workspace := range state.Datasets {
		c.datasets[workspace.SetID] = cloneDataset(workspace)
		// Existing registries have only the current release. Preserve that
		// accepted release on the first write; never invent older versions.
		c.loadVersion(workspace)
	}
	for _, version := range state.DatasetVersions {
		c.loadVersion(version)
	}
}

func (c *service) loadVersion(def Dataset) {
	if c.versions[def.SetID] == nil {
		c.versions[def.SetID] = map[int]Dataset{}
	}
	def.Retired = false // retirement is current policy, not versioned content
	c.versions[def.SetID][def.Revision] = cloneDataset(def)
}

func (c *service) datasetVersions() []Dataset {
	var out []Dataset
	for _, versions := range c.versions {
		for _, def := range versions {
			out = append(out, cloneDataset(def))
		}
	}
	return normalizeDatasetVersions(out)
}

func (c *service) persist(state State, message string) error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a service read view cannot persist changes")
	}
	head, err := c.registry.saveExpected(c.registryHead, state, message, c.as, c.requestID, c.ruleID)
	if err == nil {
		c.registryHead = head
		c.loadState(state)
	}
	cmd, _, _ := strings.Cut(strings.TrimSpace(message), " ")
	if cmd == "" {
		cmd = "persist"
	}
	return observe(c.observer, cmd, map[string]any{
		"authorityId": c.registry.AuthorityID(),
		"message":     message,
	}, err)
}

func (c *service) SetObserver(j Observer) { c.mu.Lock(); defer c.mu.Unlock(); c.observer = j }

func (c *service) SetStamp(as, requestID, ruleID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	return observe(c.observer, cmd, refs, err)
}

// RecordCreated writes registry.yaml so service.Log has a birth commit (empty registry is otherwise only git "root").
func (c *service) RecordCreated() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.persist(c.dumpState(), "init "+c.registry.AuthorityID())
}
