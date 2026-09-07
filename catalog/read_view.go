package catalog

import "kc/internal/journal"

// ReadView freezes this Catalog's currently accepted registry state for one
// read request. It shares Snapshot/Registry access, but owns its maps, journal,
// and stamps. It does not reload authority or subscribe to advancement events.
func (c *Catalog) ReadView(j journal.Journal) *Catalog {
	c.mu.RLock()
	defer c.mu.RUnlock()
	view := &Catalog{store: c.store, registry: c.registry, registryHead: c.registryHead, journal: j, readOnly: true}
	view.loadState(c.dumpState())
	return view
}
