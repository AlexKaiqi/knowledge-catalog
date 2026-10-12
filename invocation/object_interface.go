package invocation

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"kc/invoke"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/snapshot/dataset"
)

// InvokeObjectInterface shares the authorized, pinned consumption seam for
// every installed object capability. No business type selects its handler.
func InvokeObjectInterface(cx *Invocation, operation string) (any, error) {
	input, err := invocationInput(cx)
	if err != nil {
		return nil, err
	}
	return OnTarget(cx, func(declarations *reader.Serving, _ dataset.Service) (any, error) {
		values, err := declarations.ReadAddress(knowledge.Address{Kind: knowledge.KindMember, ObjectID: knowledge.ObjectID(cx.Flag("object")), AspectName: knowledge.ObjectInterfacesAspect, MemberKey: cx.Flag("implementation")})
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface implementation resolved to %d values; choose one fixed repository", len(values))
		}
		return invokeObjectInterfaceAt(cx, declarations, values[0].Repository, operation, input)
	}, func(repository kernel.RepositoryID, commit kernel.CommitID) (any, error) {
		repositories := map[kernel.RepositoryID]kernel.CommitID{repository: commit}
		declarations := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) {
			return cx.WS.Reader.Require(id, kernel.ErrKnowledgeRefUnresolved)
		}, reader.KnowledgeSetPin{SetID: "interface", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
		return invokeObjectInterfaceAt(cx, declarations, repository, operation, input)
	})
}

func invokeObjectInterfaceAt(cx *Invocation, declarations *reader.Serving, repository kernel.RepositoryID, operation string, input any) (any, error) {
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	implementation, err := declarations.ResolveObjectInterfaceAt(repository, knowledge.ObjectID(cx.Flag("object")), cx.Flag("implementation"))
	if err != nil {
		return nil, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	accessor, _ := lookup.(serving.ResourceOperationAccessor)
	registry, err := serving.WithLiveAccess(cx.ObjectInterfaces, implementation, accessor)
	if err != nil {
		return nil, err
	}
	service := serving.OpenInterfaces(declarations, registry, request)
	service.SetAuthorizer(func(_ context.Context, _ serving.InterfaceCall) error {
		observation := invoke.NoObservationTelemetry(cx.Observation)
		return Authorize(cx.Home, "resource.access", AuthorizationFlags(cx), observation.Authorization, cx.WS)
	})
	interfaceRequest := serving.InterfaceRequest{Repository: repository, Object: knowledge.ObjectID(cx.Flag("object")), Implementation: cx.Flag("implementation"), Operation: operation, Input: input}
	result, err := service.Call(cx.Context, interfaceRequest)
	if err != nil {
		return nil, err
	}
	return ObserveObjectInterfaceCall(result, interfaceRequest), nil
}

func invocationInput(cx *Invocation) (any, error) {
	raw, err := cx.Require("input")
	if err != nil {
		return nil, err
	}
	var input any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must be one JSON value: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must contain exactly one JSON value")
	}
	return input, nil
}
