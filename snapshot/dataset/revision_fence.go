package dataset

import (
	"sync"

	"kc/kernel"
)

// revisionFence orders work that must stay bound to one Dataset revision
// against publication and retirement of that same Dataset. It is keyed by
// Dataset, never held by readers, and taken before the registry commit lock,
// so unrelated Datasets and every read keep running.
type revisionFence struct {
	locks sync.Map // setID -> *sync.Mutex
}

func (f *revisionFence) lock(setID string) func() {
	value, _ := f.locks.LoadOrStore(setID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// WithCurrentRevision runs fn while revision stays the current, unretired
// revision of setID: a publication or retirement of setID that reaches its
// commit meanwhile waits until fn returns. A Dataset that is absent, retired
// or already at another revision fails with PRECONDITION_FAILED and fn does
// not run. fn must not publish or retire setID.
func (c *service) WithCurrentRevision(setID string, revision int, fn func() error) error {
	unlock := c.fence.lock(setID)
	defer unlock()
	current, ok := c.accepted().datasets[setID]
	switch {
	case !ok:
		return kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s is not defined in this service", setID)
	case current.Retired:
		return kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s is retired", setID)
	case current.Revision != revision:
		return kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s current revision is v%d, not v%d", setID, current.Revision, revision)
	}
	if fn == nil {
		return nil
	}
	return fn()
}

// commitDataset is commit for a write that can move or retire the current
// revision of setID; it waits for WithCurrentRevision holders of that Dataset.
func (c *service) commitDataset(setID, message string, mutate func(*acceptedState) (State, error)) error {
	if c.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "a service read view cannot persist changes")
	}
	unlock := c.fence.lock(setID)
	defer unlock()
	return c.commit(message, mutate)
}
