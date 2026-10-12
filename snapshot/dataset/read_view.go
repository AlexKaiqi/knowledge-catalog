package dataset

// ReadView freezes this service's currently accepted registry state for one
// read request. It shares Snapshot/registry access, but owns its maps, journal,
// and stamps. It does not reload authority or subscribe to advancement events.
func (c *service) ReadView(j Observer) Service {
	c.mu.RLock()
	defer c.mu.RUnlock()
	view := &service{store: c.store, registry: c.registry, registryHead: c.registryHead, observer: j, readOnly: true}
	view.loadState(c.dumpState())
	return view
}
