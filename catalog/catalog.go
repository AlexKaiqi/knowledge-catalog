package catalog

import (
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/repository"
)

// Catalog is combination over a set of Repositories.
//
//	ViewDefinition — consumer recipe: which repos, which published selector
//	Serving        — OpenView resolves those selectors at open (not persisted)
//
// Operations, by what they change:
//
//	recipe:   DefineView, View, RetireDefinition
//	serving:  OpenView / ResolveView / FederatedRead / CheckResolved / PlanIndex
//	register: RegisterRepository
//	space:    Archive
//	history:  Log
//	hooks:    AddHook / NotifySnapshot (in-process; not outbound kc hook-add)
type Catalog struct {
	store        *repository.Store
	registry     *Registry
	views        map[string]ViewDefinition
	repositories map[string]struct{}
	archived     bool
	journal      journal.Journal
	as           string
	requestID    string
	ruleID       string
	hooks        []Hook
}

func NewCatalog(store *repository.Store, registry *Registry) (*Catalog, error) {
	if registry == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "catalog registry is required")
	}
	c := &Catalog{
		store:        store,
		registry:     registry,
		views:        map[string]ViewDefinition{},
		repositories: map[string]struct{}{},
	}
	state, err := registry.Load()
	if err != nil {
		return nil, err
	}
	c.LoadState(state)
	if store != nil {
		store.OnSnapshot(func(ev repository.Snapshot) {
			c.NotifySnapshot(Snapshot{
				Repository: ev.Repository,
				From:       ev.From,
				To:         ev.To,
				ObjectIDs:  ev.ObjectIDs,
			})
		})
	}
	return c, nil
}

// DumpState snapshots the registry maps. Not a dump of those Repositories' objects.
func (c *Catalog) DumpState() CatalogState {
	views := make([]ViewDefinition, 0, len(c.views))
	for _, view := range c.views {
		views = append(views, view)
	}
	return CatalogState{
		Views:        views,
		Repositories: c.Repositories(),
		Archived:     c.archived,
		CatalogID:    c.registry.CatalogID(),
	}
}

// LoadState replaces the maps from registry bytes. Called at construct.
func (c *Catalog) LoadState(state CatalogState) {
	c.views = map[string]ViewDefinition{}
	c.repositories = map[string]struct{}{}
	c.archived = state.Archived
	for _, id := range state.Repositories {
		c.repositories[id] = struct{}{}
	}
	for _, view := range state.Views {
		c.views[view.ViewID] = view
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
