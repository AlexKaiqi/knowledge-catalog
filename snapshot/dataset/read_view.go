package dataset

// ReadView freezes this service's currently accepted registry version for one
// read request. It shares Snapshot/registry access and the immutable version
// itself (no copy, no lock), but owns its journal and stamps. A later
// publication swaps the shared pointer, never this view's version. It does not
// reload authority or subscribe to advancement events.
func (c *service) ReadView(j Observer) Service {
	view := &service{
		registryState: &registryState{store: c.store, registry: c.registry},
		observer:      j,
		readOnly:      true,
	}
	view.current.Store(c.accepted())
	return view
}

// RequestView serves one mutating request. It shares the accepted registry
// state and its commit lock with every other view of this authority, so
// a publication is immediately visible to later requests, while the evidence
// observer and stamp belong to this request alone.
func (c *service) RequestView(j Observer, as, requestID, ruleID string) Service {
	return &service{
		registryState: c.registryState,
		readOnly:      c.readOnly,
		observer:      j,
		as:            as,
		requestID:     requestID,
		ruleID:        ruleID,
	}
}
