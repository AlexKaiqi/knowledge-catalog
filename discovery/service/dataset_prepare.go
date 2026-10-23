package service

import (
	"context"
	sourcebridge "kc/liveaccess/discovery"

	index "kc/discovery/projection"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
)

// DatasetPreparation is the two half-steps of explicit preparation: build the
// accepted revision's corpus, then republish the observation of that corpus.
// Both are answered by the index instance that owns every source, so they stay
// one port rather than two independent capabilities.
type DatasetPreparation interface {
	PrepareAcceptedDataset(context.Context, reader.DatasetPin) error
	RefreshPreparedDatasetState(context.Context, reader.MemberLookup, reader.DatasetPin, index.ObservationSource) (index.StateSync, error)
}

type DatasetPrepareRequest struct {
	Pin          reader.DatasetPin
	Lookup       reader.MemberLookup
	State        objectserving.StateLookup
	StateRequest ProjectionStateRequest
}

type DatasetPrepareResult struct {
	Observation index.StateSync
}

type DatasetPrepareExecutor struct {
	Plugins DatasetPreparation
}

// DatasetPrepareExecutor builds one accepted revision on request. Like the
// projection executors beside it, it is the only place a caller's LiveAccess
// runtime becomes an observation source: callers hand over the runtime and the
// identity factory, and this layer decides whether an observation can run.
func (e DatasetPrepareExecutor) Execute(ctx context.Context, request DatasetPrepareRequest) (DatasetPrepareResult, error) {
	if e.Plugins == nil {
		return DatasetPrepareResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"Dataset preparation service is unavailable")
	}
	if err := e.Plugins.PrepareAcceptedDataset(ctx, request.Pin); err != nil {
		return DatasetPrepareResult{}, err
	}
	source, err := datasetObservationSource(request)
	if err != nil {
		return DatasetPrepareResult{}, err
	}
	observation, err := e.Plugins.RefreshPreparedDatasetState(ctx, request.Lookup, request.Pin, source)
	if err != nil {
		return DatasetPrepareResult{}, err
	}
	return DatasetPrepareResult{Observation: observation}, nil
}

// datasetObservationSource binds the runtime this process has. A caller with no
// runtime at all contributes no source: the instance then answers with its own
// worker or reports that observations cannot be refreshed
// (doc/design/discovery.md#观察).
func datasetObservationSource(request DatasetPrepareRequest) (index.ObservationSource, error) {
	if request.State == nil {
		return nil, nil
	}
	var observationRequest objectmodel.RequestContext
	if request.StateRequest != nil {
		var err error
		observationRequest, err = request.StateRequest()
		if err != nil {
			return nil, err
		}
	}
	return sourcebridge.Source{Lookup: request.State, Request: observationRequest}, nil
}
