package catalog

import (
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/snapshot"
)

// Catalog is combination over a set of Repositories.
//
//	WorkspaceDefinition — consumer recipe: which repos, which published selector
//	ResolvedWorkspace   — ResolveWorkspace maps those selectors to fixed commits at open
//
// Catalog is not a file warehouse (that is snapshot.Store) and not a knowledge
// protocol. Knowledge wrapping lives in writer/reader/index. Dynamic State/Stream
// observation belongs to an upper-layer Materialization runtime; this package
// freezes only Repository commits.
//
// Operations, by what they change:
//
//	recipe:   DefineWorkspace, Workspace, RetireWorkspace
//	resolve:  ResolveWorkspace / CheckResolved
//	register: RegisterRepository
//	space:    Archive
//	history:  Log
//	hooks:    AddHook / NotifySnapshot (in-process; not outbound kc hook-add)
//
// object_id is not a Catalog concern. Consumer Read / AccessSpec live in reader/.
type Catalog struct {
	store        *snapshot.Registry
	registry     *Registry
	workspaces   map[string]WorkspaceDefinition
	repositories map[string]struct{}
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
		workspaces:   map[string]WorkspaceDefinition{},
		repositories: map[string]struct{}{},
	}
	state, err := registry.Load()
	if err != nil {
		return nil, err
	}
	c.LoadState(state)
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
	workspaces := make([]WorkspaceDefinition, 0, len(c.workspaces))
	for _, workspace := range c.workspaces {
		workspaces = append(workspaces, workspace)
	}
	return CatalogState{
		Workspaces:   workspaces,
		Repositories: c.Repositories(),
		Archived:     c.archived,
		CatalogID:    c.registry.CatalogID(),
	}
}

// LoadState replaces the maps from registry bytes. Called at construct.
func (c *Catalog) LoadState(state CatalogState) {
	c.workspaces = map[string]WorkspaceDefinition{}
	c.repositories = map[string]struct{}{}
	c.archived = state.Archived
	for _, id := range state.Repositories {
		c.repositories[id] = struct{}{}
	}
	for _, workspace := range state.Workspaces {
		c.workspaces[workspace.WorkspaceID] = workspace
	}
}

func (c *Catalog) persist(message string) error {
	err := c.registry.Save(c.DumpState(), message, c.as, c.requestID, c.ruleID)
	cmd, _, _ := strings.Cut(strings.TrimSpace(message), " ")
	if cmd == "" {
		cmd = "persist"
	}
	return journal.Finish(c.journal, journal.LayerSystem, "catalog", cmd, map[string]any{
		"catalogId": c.registry.CatalogID(),
		"message":   message,
	}, err)
}

func (c *Catalog) SetJournal(j journal.Journal) { c.journal = j }

func (c *Catalog) SetStamp(as, requestID, ruleID string) {
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
	return c.persist("init " + c.registry.CatalogID())
}
