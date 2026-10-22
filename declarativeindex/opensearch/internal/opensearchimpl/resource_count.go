package opensearchimpl

import (
	"context"
	"reflect"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// Earlier owned resources recorded the build's initial count (zero), while
// the accepted READY pointer and immutable retained record have the final
// count. Read that exact generation's metadata; never borrow the current
// generation's count or rewrite ownership during a query.
func (e *openSearchEngine) restoreResourceCount(ctx context.Context, record controlDoc) (controlDoc, error) {
	if record.State != "RESOURCE" || record.ObjectCount != 0 {
		return record, nil
	}
	retained, version, err := e.loadControlRecord(ctx, e.controlID+"-retained-"+record.Generation)
	if err != nil {
		return controlDoc{}, err
	}
	if version == nil {
		retained, version, err = e.loadControlContext(ctx)
		if err != nil {
			return controlDoc{}, err
		}
		if version == nil || retained.State != ports.ProjectionStateReady {
			return controlDoc{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "legacy resource has no fixed count authority")
		}
	} else if retained.State != "RETAINED_GENERATION" {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "legacy count authority has different ownership")
	}
	retainedReady, resourceReady := retained, record
	retainedReady.State, resourceReady.State = ports.ProjectionStateReady, ports.ProjectionStateReady
	if retained.Repository != record.Repository || retained.ActiveIndex != record.ActiveIndex || !reflect.DeepEqual(metaFromControl(retainedReady), metaFromControl(resourceReady)) || retained.ObjectCount < 0 {
		return controlDoc{}, kernel.Fail(kernel.ErrPreconditionFailed, "legacy count authority differs from fixed resource")
	}
	record.ObjectCount = retained.ObjectCount
	return record, nil
}
