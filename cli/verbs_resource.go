package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
)

func resourceVerbs() map[string]command {
	return map[string]command{
		"resource-access": {stage: stageGoverned, run: verbResourceAccess},
	}
}

// verbResourceAccess invokes the separately configured resource-access/v1
// runtime from either an Aspect Binding (--aspect) or a pinned
// ResourceDescriptor operation (--operation + --input).
func verbResourceAccess(cx *invocation) (any, error) {
	if operation := strings.TrimSpace(cx.flag("operation")); operation != "" {
		return accessResourceOperation(cx, operation)
	}
	if cx.flag("input") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "knowledge access --input requires --operation")
	}
	resolved, err := verbResolveBinding(cx)
	if err != nil {
		return nil, err
	}
	lookup, err := resourceLookup(cx)
	if err != nil {
		return nil, err
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

func resourceLookup(cx *invocation) (knowledgeserving.StateLookup, error) {
	if cx.State != nil {
		return cx.State, nil
	}
	origin := strings.TrimSpace(os.Getenv("KC_RESOURCE_ACCESS_URL"))
	if origin == "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource-access/v1 runtime is not configured")
	}
	return NewHTTPStateLookup(origin, nil)
}

func accessResourceOperation(cx *invocation, operation string) (any, error) {
	if cx.flag("aspect") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "knowledge access uses either --aspect Binding hydration or --operation descriptor access")
	}
	rawInput, err := cx.require("input")
	if err != nil {
		return nil, err
	}
	var input any
	decoder := json.NewDecoder(strings.NewReader(rawInput))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must be one JSON value: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must contain exactly one JSON value")
	}

	descriptor, err := pinnedResourceDescriptor(cx)
	if err != nil {
		return nil, err
	}
	value, ok := descriptor.Value.(map[string]any)
	if !ok || stringValue(value["kind"]) != "ResourceDescriptor" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object %s is not a ResourceDescriptor", descriptor.ObjectID)
	}
	runtime := strings.TrimSpace(stringValue(value["runtime"]))
	protocol := strings.TrimSpace(stringValue(value["protocol"]))
	access, ok := value["access"].(map[string]any)
	if !ok || runtime == "" || protocol == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires runtime, protocol and access", descriptor.ObjectID)
	}
	operationValue, ok := access[operation].(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare operation %s", descriptor.ObjectID, operation)
	}
	call := strings.TrimSpace(stringValue(operationValue["call"]))
	if call == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operation %s requires call", descriptor.ObjectID, operation)
	}
	lookup, err := resourceLookup(cx)
	if err != nil {
		return nil, err
	}
	accessor, ok := lookup.(resourceOperationAccessor)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource-access/v1 runtime does not support descriptor operations")
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
	return accessor.AccessResource(cx.Context, resourceOperationRequest{
		Descriptor: resourceDescriptorCoordinate{
			ObjectID: descriptor.ObjectID, Repository: descriptor.Repository, Commit: descriptor.Commit,
		},
		Runtime: runtime, Protocol: protocol, Operation: operation, Call: call, Input: input,
		Identity: stateRuntimeIdentity{
			Principal: identity.Principal, OnBehalfOf: identity.OnBehalfOf, RequestID: requestID,
			TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		},
	})
}

type pinnedDescriptor struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	ObjectID   knowledge.ObjectID
	Value      any
}

func pinnedResourceDescriptor(cx *invocation) (pinnedDescriptor, error) {
	result, err := verbRead(cx)
	if err != nil {
		return pinnedDescriptor{}, err
	}
	switch value := result.(type) {
	case []knowledgeserving.ReadResult:
		if len(value) != 1 {
			return pinnedDescriptor{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
				"ResourceDescriptor %s resolved to %d values", cx.flag("object"), len(value))
		}
		return pinnedDescriptor{
			Repository: value[0].Repository, Commit: value[0].Commit,
			ObjectID: value[0].ObjectID, Value: value[0].Value,
		}, nil
	case knowledge.KnowledgeValue:
		return pinnedDescriptor{
			Repository: value.Repository, Commit: value.Commit,
			ObjectID: value.Address.ObjectID, Value: value.Value,
		}, nil
	default:
		return pinnedDescriptor{}, fmt.Errorf("ResourceDescriptor read returned %T", result)
	}
}
