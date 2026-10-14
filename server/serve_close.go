package server

import (
	"context"
	"time"
)

// BeginShutdown refuses new requests and cancels background maintenance while
// retaining resources needed by accepted HTTP requests. The host can then
// drain its HTTP server before Close releases those resources and telemetry.
func (h *ManagedHTTPHandler) BeginShutdown() {
	if h == nil {
		return
	}
	h.beginOnce.Do(func() {
		h.lifeMu.Lock()
		h.closed = true
		h.lifeMu.Unlock()
		if h.stopHome != nil {
			h.stopHome()
		}
	})
}

// Close cancels maintenance, drains typed tasks and releases their resources,
// then shuts down the process telemetry owned by this handler.
func (h *ManagedHTTPHandler) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		h.BeginShutdown()
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
