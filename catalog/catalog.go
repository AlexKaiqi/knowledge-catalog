package catalog

import (
	"strings"
	"sync"

	"kc/internal/journal"
	"kc/kernel"
	"kc/snapshot"
)

// Catalog is combination over a set of Repositories.
//
//	KnowledgeSet — published Dataset: file pointers frozen to commits at define
//	ResolvedKnowledgeSet   — ResolveKnowledgeSet returns that version's file list and implied {Repository → commit}
//
// Catalog is not a file warehouse (that is snapshot.Store) and not a knowledge
// protocol. Knowledge wrapping lives in knowledge/{writer,reader} and index. Dynamic State/Stream
// observation belongs to an upper-layer Materialization runtime; this package
// freezes only Repository commits.
//
// Operations, by what they change:
//
//	recipe:   DefineKnowledgeSet, Workspace, RetireKnowledgeSet
//	resolve:  ResolveKnowledgeSet / CheckResolved
//	register: RegisterRepository
//	space:    Archive
//	history:  Log
//	hooks:    AddHook / NotifySnapshot (in-process; not outbound kc hook-add)
//
// Host git checkout / sync / status live in catalog/worktree. They consume
// recipes and pins but are not registry mutations.
//
// object_id is not a Catalog concern. Consumer Read lives in knowledge/reader;
// AccessSpec lives in retrieval/.
type Catalog struct {
	mu           sync.RWMutex
	store        *snapshot.Registry
	registry     *Registry
	registryHead string
	datasets     map[string]KnowledgeSet
	versions     map[string]map[int]KnowledgeSet
	repositories map[string]struct{}
	readOnly     bool
	archived     bool
	journal      journal.Journal
	as           string
	requestID    string
	ruleID       string
	hooks        []Hook
}

func NewCatalog(store *snapshot.Registry, registry *Registry) (*Catalog, error) {
	if registry == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog registry is required")
	}
	c := &Catalog{
		store:        store,
		registry:     registry,
		datasets:     map[string]KnowledgeSet{},
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
func (c *Catalog) DumpState() CatalogState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dumpState()
}

func (c *Catalog) dumpState() CatalogState {
	workspaces := make([]KnowledgeSet, 0, len(c.datasets))
	for _, workspace := range c.datasets {
		workspaces = append(workspaces, cloneKnowledgeSet(workspace))
	}
	return CatalogState{
		KnowledgeSets:   workspaces,
		DatasetVersions: c.datasetVersions(),
		Repositories:    c.repositoriesList(),
		Archived:        c.archived,
		CatalogID:       c.registry.CatalogID(),
	}
}

// LoadState replaces the maps from registry bytes. Called at construct.
func (c *Catalog) LoadState(state CatalogState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadState(state)
}

func (c *Catalog) loadState(state CatalogState) {
	c.datasets = map[string]KnowledgeSet{}
	c.versions = map[string]map[int]KnowledgeSet{}
	c.repositories = map[string]struct{}{}
	c.archived = state.Archived
	for _, id := range state.Repositories {
		c.repositories[id] = struct{}{}
	}
	for _, workspace := range state.KnowledgeSets {
		c.datasets[workspace.SetID] = cloneKnowledgeSet(workspace)
		// Existing registries have only the current release. Preserve that
		// accepted release on the first write; never invent older versions.
		c.loadVersion(workspace)
	}
	for _, version := range state.DatasetVersions {
		c.loadVersion(version)
	}
}

func (c *Catalog) loadVersion(def KnowledgeSet) {
	if c.versions[def.SetID] == nil {
		c.versions[def.SetID] = map[int]KnowledgeSet{}
	}
	def.Retired = false // retirement is current policy, not versioned content
	c.versions[def.SetID][def.Revision] = cloneKnowledgeSet(def)
}

func (c *Catalog) datasetVersions() []KnowledgeSet {
	var out []KnowledgeSet
	for _, versions := range c.versions {
		for _, def := range versions {
			out = append(out, cloneKnowledgeSet(def))
		}
	}
	return normalizeDatasetVersions(out)
}

func (c *Catalog) persist(state CatalogState, message string) error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a Catalog read view cannot persist changes")
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
	return journal.Finish(c.journal, journal.LayerSystem, "catalog", cmd, map[string]any{
		"catalogId": c.registry.CatalogID(),
		"message":   message,
	}, err)
}

func (c *Catalog) SetJournal(j journal.Journal) { c.mu.Lock(); defer c.mu.Unlock(); c.journal = j }

func (c *Catalog) SetStamp(as, requestID, ruleID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.as = as
	c.requestID = requestID
	c.ruleID = ruleID
}

func (c *Catalog) note(cmd string, refs map[string]any, err error) error {
	if refs == nil {
		refs = map[string]any{}
	}
	if _, ok := refs["catalogId"]; !ok && c.registry != nil {
		refs["catalogId"] = c.registry.CatalogID()
	}
	return journal.Finish(c.journal, journal.LayerSystem, "catalog", cmd, refs, err)
}

// RecordCreated writes catalog.yaml so Catalog.Log has a birth commit (empty registry is otherwise only git "root").
func (c *Catalog) RecordCreated() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.persist(c.dumpState(), "init "+c.registry.CatalogID())
}
