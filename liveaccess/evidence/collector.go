package evidence

import (
	"context"
	"encoding/json"
	"sync"

	"kc/kernel"
)

// Collector belongs to one consumer request. Capturing is explicit: projection
// maintenance and units excluded by a read selector do not create read evidence.
type Collector struct {
	mu      sync.Mutex
	samples [][]byte
}

type collectorKey struct{}

func NewCollector() *Collector { return &Collector{} }

func WithCollector(ctx context.Context, collector *Collector) context.Context {
	return context.WithValue(ctx, collectorKey{}, collector)
}

// Capture freezes an adopted result against subsequent caller mutation. A
// context without a collector is an ordinary non-consumer operation.
func Capture(ctx context.Context, sample Sample) error {
	collector, _ := ctx.Value(collectorKey{}).(*Collector)
	if collector == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.samples = append(collector.samples, raw)
	return nil
}

// Values returns independent copies of the results captured by this request.
func (c *Collector) Values() ([]Sample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	values := make([]Sample, len(c.samples))
	for i, raw := range c.samples {
		if err := kernel.UnmarshalJSON(raw, &values[i]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// CapturedValues reads only the collector bound to this consumer request.
func CapturedValues(ctx context.Context) ([]Sample, error) {
	collector, _ := ctx.Value(collectorKey{}).(*Collector)
	if collector == nil {
		return nil, nil
	}
	return collector.Values()
}
