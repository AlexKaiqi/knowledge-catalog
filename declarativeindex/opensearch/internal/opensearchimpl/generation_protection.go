package opensearchimpl

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"

	"kc/kernel"
)

// A replaced pointer is not a lifecycle authority. Retain the immutable old
// generation before changing that pointer so a restart cannot lose its owner
// or reinterpret a process-local PIT timer as permission to delete it.
// A lifecycle reconciler must resolve these records before releasing resources.
func (s *rebuildSession) preservePreviousGeneration() error {
	if s.old.ActiveIndex == "" || s.old.ActiveIndex == s.physicalIndex {
		return nil
	}
	record := s.old
	// Deadlines belong to the current admission authority, not immutable
	// generation metadata; pruning them must not change a retained identity.
	record.ObservationDeadlines = nil
	record.State = "RETAINED_GENERATION"
	path := "/" + s.engine.controlIndexPath() + "/_doc/" + url.PathEscape(s.engine.controlID+"-retained-"+s.old.Generation)
	status, body, err := s.engine.do(http.MethodPut, path+"?op_type=create&refresh=true", record)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		status, body, err = s.engine.do(http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		var existing struct {
			Source controlDoc `json:"_source"`
		}
		if status == http.StatusOK && json.Unmarshal(body, &existing) == nil && reflect.DeepEqual(existing.Source, record) {
			return nil
		}
		return kernel.Fail(kernel.ErrPreconditionFailed, "retained generation ownership differs for %s", s.old.ActiveIndex)
	}
	if status >= http.StatusMultipleChoices {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "cannot protect previous generation %s: HTTP %d: %s", s.old.ActiveIndex, status, body)
	}
	return nil
}
