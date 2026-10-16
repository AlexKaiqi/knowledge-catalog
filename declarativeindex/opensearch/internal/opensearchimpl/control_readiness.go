package opensearchimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Another engine of this namespace can expose its newly created control index
// while its primary is still recovering. Only that explicit state waits; no
// operation creates an index or retries unrelated provider failures here.
func (e *openSearchEngine) readControlResponse(ctx context.Context, path string) (int, []byte, error) {
	status, body, err := e.doContext(ctx, http.MethodGet, path, nil)
	if !controlPrimaryRecovering(status, body, e.controlIndexPath()) {
		return status, body, err
	}
	recovery, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for controlPrimaryRecovering(status, body, e.controlIndexPath()) {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-recovery.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return 0, nil, ctx.Err()
			}
			return status, body, err
		case <-timer.C:
		}
		status, body, err = e.doContext(recovery, http.MethodGet, path, nil)
	}
	if err != nil && ctx.Err() != nil {
		return 0, nil, ctx.Err()
	}
	return status, body, err
}

func controlPrimaryRecovering(status int, body []byte, index string) bool {
	if status != http.StatusServiceUnavailable {
		return false
	}
	var response struct {
		Error struct {
			Type     string `json:"type"`
			CausedBy struct {
				Type   string `json:"type"`
				Index  string `json:"index"`
				Reason string `json:"reason"`
			} `json:"caused_by"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	cause := response.Error.CausedBy
	return response.Error.Type == "no_shard_available_action_exception" && cause.Type == "illegal_index_shard_state_exception" && cause.Index == index && strings.HasPrefix(cause.Reason, "CurrentState[RECOVERING]")
}
