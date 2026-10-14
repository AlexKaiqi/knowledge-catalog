package control

import (
	"context"

	index "kc/discovery/projection"
	"kc/kernel"
)

// ApplyNotice synchronously refreshes this notice alone. A configured worker
// source keeps its own identity; otherwise requestSource is borrowed only for
// this call. It is never retained or used to drain unrelated pending work.
func (c *Controller) ApplyNotice(ctx context.Context, notice index.ChangeNotice, requestSource index.ObservationSource) (index.StateSync, error) {
	if err := index.ValidateChangeNotice(notice); err != nil {
		return index.StateSync{}, err
	}
	if c.index == nil || c.lookup == nil {
		return index.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection notice requires a configured index and repository lookup")
	}
	c.catchupMu.Lock()
	defer c.catchupMu.Unlock()
	if err := ctx.Err(); err != nil {
		return index.StateSync{}, err
	}
	if err := c.refreshDemands(ctx); err != nil {
		return index.StateSync{}, err
	}
	if _, active := c.demands[notice.Repository]; !active {
		return index.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection notice repository has no effective maintenance demand")
	}
	source := c.stateRuntime()
	if source == nil {
		source = requestSource
	} else {
		ctx = workerObservationContext{ctx}
	}
	if source == nil {
		return index.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection notice requires a Materialization Runtime")
	}
	return c.refreshNotice(ctx, notice, source)
}

// A worker borrows cancellation and deadlines, never request identity values.
// Its configured index.ObservationSource supplies its independent execution context.
type workerObservationContext struct{ context.Context }

func (workerObservationContext) Value(any) any { return nil }
