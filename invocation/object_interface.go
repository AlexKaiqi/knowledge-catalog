package invocation

import (
	"context"
	"encoding/json"
	"io"
	"kc/interfaces"
	interfaceservice "kc/interfaces/service"
	"strings"

	"kc/invoke"
	"kc/kernel"
	serving "kc/liveaccess"
	accesscontract "kc/liveaccess/contract"
	objectmodel "kc/object"
	"kc/object/reader"
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
		values, err := declarations.ReadAddress(objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: objectmodel.ObjectID(cx.Flag("object")), AspectName: interfaces.ObjectInterfacesAspect, MemberKey: cx.Flag("implementation")})
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "interface implementation resolved to %d values; choose one fixed repository", len(values))
		}
		return invokeObjectInterfaceAt(cx, declarations, values[0].Repository, operation, input)
	}, func(repository kernel.RepositoryID, commit kernel.CommitID) (any, error) {
		declarations := repositoryServingAt(cx, repository, commit, "interface")
		return invokeObjectInterfaceAt(cx, declarations, repository, operation, input)
	})
}

func invokeObjectInterfaceAt(cx *Invocation, declarations *reader.Serving, repository kernel.RepositoryID, operation string, input any) (any, error) {
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	implementation, err := interfaces.ResolveObjectInterfaceAt(declarations, repository, objectmodel.ObjectID(cx.Flag("object")), cx.Flag("implementation"))
	if err != nil {
		return nil, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	accessor, _ := lookup.(serving.ResourceOperationAccessor)
	registry := cx.ObjectInterfaces
	if implementation.Contract.Object == accesscontract.LiveAccessInterfaceV1 && cx.WS.LiveAccess != nil {
		registry, err = cx.WS.LiveAccess.InterfaceRegistryFor(implementation)
	} else {
		registry, err = serving.WithLiveAccess(registry, implementation, accessor)
	}
	if err != nil {
		return nil, err
	}
	service := interfaceservice.Open(declarations, registry, request)
	service.SetAuthorizer(func(_ context.Context, _ interfaces.InterfaceCall) error {
		observation := invoke.NoObservationTelemetry(cx.Observation)
		return Authorize(cx.Home, "resource.access", AuthorizationFlags(cx), observation.Authorization, cx.WS)
	})
	interfaceRequest := interfaces.InterfaceRequest{Repository: repository, Object: objectmodel.ObjectID(cx.Flag("object")), Implementation: cx.Flag("implementation"), Operation: operation, Input: input}
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
