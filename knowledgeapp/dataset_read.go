package knowledgeapp

import (
	"context"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
)

type DatasetReadRequest struct {
	Object   knowledge.ObjectID
	Address  *knowledge.Address
	Selector *knowledge.AspectSelector
}
type DatasetReadExecutor struct {
	Authorize func(context.Context) error
	Resolve   func(context.Context) (*knowledgeserving.Service, error)
	Deliver   func(context.Context, []knowledgeserving.ReadResult) ([]knowledgeserving.ReadResult, error)
}

func (e DatasetReadExecutor) Execute(ctx context.Context, request DatasetReadRequest) ([]knowledgeserving.ReadResult, error) {
	if e.Authorize == nil || e.Resolve == nil || e.Deliver == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset read services are incomplete")
	}
	if request.Object == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "read requires an object")
	}
	if request.Address != nil && request.Address.ObjectID != request.Object {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "read address does not match object")
	}
	if err := e.Authorize(ctx); err != nil {
		return nil, err
	}
	logical, err := e.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	if logical == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset serving is unavailable")
	}
	var values []knowledgeserving.ReadResult
	if request.Address != nil {
		values, err = logical.ReadAddress(ctx, *request.Address)
	} else {
		values, err = logical.Read(ctx, request.Object, request.Selector)
	}
	if err != nil {
		return nil, err
	}
	return e.Deliver(ctx, values)
}
