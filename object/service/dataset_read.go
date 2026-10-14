package service

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
)

type DatasetReadRequest struct {
	Reference objectmodel.Reference
}

type DatasetReadExecutor struct {
	Authorize     func(context.Context) error
	ReferenceRead func(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error)
	Deliver       func(context.Context, []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error)
}

func (e DatasetReadExecutor) Execute(ctx context.Context, request DatasetReadRequest) ([]objectmodel.ObservedValue, error) {
	if e.Authorize == nil || e.Deliver == nil || e.ReferenceRead == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset read services are incomplete")
	}
	if _, err := objectmodel.FormatReference(request.Reference); err != nil {
		return nil, err
	}
	if err := e.Authorize(ctx); err != nil {
		return nil, err
	}
	values, err := e.ReferenceRead(ctx, request.Reference)
	if err != nil {
		return nil, err
	}
	return e.Deliver(ctx, values)
}
