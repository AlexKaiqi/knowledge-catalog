package control

import "context"

// DatasetLane is one maintenance pass over work whose target list lives above
// this package. Control decides when the pass runs and how its failure joins
// the other lanes; it does not know what a Dataset release is, because that
// type belongs to the accepted authority layer and control sits below it.
type DatasetLane func(context.Context) error

// SetDatasetLane supplies the pass that builds accepted-release corpora. A nil
// lane is absent rather than a failure: a deployment that cannot build corpora
// never installs one.
func (c *Controller) SetDatasetLane(lane DatasetLane) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	c.datasetLane = lane
}

// catchUpDatasetLane runs the assembled pass, if any. It runs after demand,
// projection and fixed-basis work. A corpus covers a whole accepted release, so
// the pass is never reduced to one Repository: scoped work keeps its own lanes.
func (c *Controller) catchUpDatasetLane(ctx context.Context) error {
	c.lookupMu.RLock()
	lane := c.datasetLane
	c.lookupMu.RUnlock()
	if lane == nil {
		return nil
	}
	return lane(ctx)
}
