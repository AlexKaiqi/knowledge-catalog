package cli

import (
	"os"
	"strings"

	"kc/kernel"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
)

func resourceVerbs() map[string]command {
	return map[string]command{
		"resource-access": {stage: stageGoverned, run: verbResourceAccess},
	}
}

// verbResourceAccess resolves the immutable Binding declaration first, then
// invokes the separately configured resource-access/v1 runtime. It currently
// supports the State lookup contract; stream and source-specific operations
// remain capabilities of their own runtime clients.
func verbResourceAccess(cx *invocation) (any, error) {
	resolved, err := verbResolveBinding(cx)
	if err != nil {
		return nil, err
	}
	lookup := cx.State
	if lookup == nil {
		origin := strings.TrimSpace(os.Getenv("KC_RESOURCE_ACCESS_URL"))
		if origin == "" {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource-access/v1 runtime is not configured")
		}
		lookup, err = NewHTTPStateLookup(origin, nil)
		if err != nil {
			return nil, err
		}
	}
	bindings := []reader.ResolvedBinding{}
	switch value := resolved.(type) {
	case reader.ResolvedBinding:
		bindings = append(bindings, value)
	case []reader.ResolvedBinding:
		bindings = append(bindings, value...)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resolved Binding has an unsupported shape")
	}
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	trace, err := traceContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	requestID, err := requestIDFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	results := make([]knowledgeserving.StateObservation, 0, len(bindings))
	for _, binding := range bindings {
		observation, err := lookup.LookupState(cx.Context, knowledgeserving.StateLookupRequest{
			Binding: binding, Identity: identity, Trace: trace, RequestID: requestID,
		})
		if err != nil {
			return nil, err
		}
		results = append(results, observation)
	}
	return map[string]any{"bindings": bindings, "observations": results}, nil
}
