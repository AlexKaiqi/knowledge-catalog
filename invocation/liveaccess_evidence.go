package invocation

import (
	"context"
	"kc/invoke"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

func captureReadRequest(ctx context.Context, command string, flags map[string]invoke.FlagValue) (context.Context, error) {
	if command != "object-read" && command != "object-access" {
		return ctx, nil
	}
	ctx, requestID, err := liveservice.BeginReadEvidence(ctx, invoke.FlagString(flags, "request-id"))
	if err == nil {
		flags["request-id"] = requestID
	}
	return ctx, err
}

func retainReadResults(ctx context.Context, home string, flags map[string]invoke.FlagValue, result any) (any, error) {
	accesses := ObjectAccesses(result)
	delivered := make([]objectmodel.PinnedObjectRef, len(accesses))
	for i, access := range accesses {
		delivered[i] = access.ObjectRef
	}
	identity, err := IdentityContextFrom(flags)
	if err != nil {
		return nil, err
	}
	trace, err := TraceContextFrom(flags)
	if err != nil {
		return nil, err
	}
	request, err := RequestIDFrom(flags)
	if err != nil {
		return nil, err
	}
	reference, err := liveservice.RetainReadEvidence(ctx, home, objectmodel.RequestContext{Identity: identity, Trace: trace, RequestID: request}, delivered)
	if err != nil {
		return nil, err
	}
	if reference == nil {
		return result, nil
	}
	observed, ok := result.(ObservedAccessResult)
	if !ok {
		observed = ObservedAccessResult{Output: result, Object: accesses}
	}
	observed.LiveAccess = reference
	return observed, nil
}
