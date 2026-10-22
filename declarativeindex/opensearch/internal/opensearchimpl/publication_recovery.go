package opensearchimpl

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	"kc/kernel"
)

// A lost response is not an authoritative rejection. Only the original
// immutable generation plus its exact intended metadata can prove adoption.
// A different current generation does not prove this one was never adopted:
// another publisher could already have replaced it while queries retain it.
func (s *rebuildSession) recoverPublication(ready controlDoc, publishErr error) error {
	current, _, readErr := s.engine.loadControl()
	if readErr == nil && current.Repository == ready.Repository && current.ActiveIndex == ready.ActiveIndex &&
		current.Generation == ready.Generation && current.ObjectCount == ready.ObjectCount &&
		reflect.DeepEqual(metaFromControl(current), metaFromControl(ready)) {
		return nil
	}
	// Preserve an independently addressable recovery record. Neither failure
	// to read the authority nor failure to retain this record permits deleting
	// the physical generation. Unowned-resource reconciliation must retain it
	// when the record cannot be confirmed.
	retained := ready
	retained.State = "PUBLISH_UNCONFIRMED"
	retained.LastError = fmt.Sprint(publishErr)
	path := "/" + s.engine.controlIndexPath() + "/_doc/" + url.PathEscape(s.engine.controlID+"-unconfirmed-"+s.generation) + "?op_type=create&refresh=true"
	status, body, retainErr := s.engine.do(http.MethodPut, path, retained)
	if retainErr == nil && status >= 300 && status != http.StatusConflict {
		retainErr = fmt.Errorf("retain publication recovery: HTTP %d: %s", status, body)
	}
	s.done = true
	s.engine.buildMu.Unlock()
	return kernel.Fail(kernel.ErrTemporaryUnavailable,
		"OpenSearch publication remains unconfirmed; resource %s retained for recovery: publication=%v authority=%v record=%v", s.physicalIndex, publishErr, readErr, retainErr)
}
