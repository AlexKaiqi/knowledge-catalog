package server

import (
	"context"
	"time"
)

// Close flushes and shuts down the process telemetry owned by this handler.
// Embedders and tests can type-assert interface{ Close() error }.
func (h *ManagedHTTPHandler) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		if h.closeHome != nil {
			h.err = h.closeHome()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.runtime.Shutdown(ctx); err != nil && h.err == nil {
			h.err = err
		}
	})
	return h.err
}
