package reader

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
)

func (r *nativeReadRepository) ScanHistory(ctx context.Context, id objectmodel.ObjectID, at kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
	port, err := nativeReadPort[objectmodel.HistoryScanner](r.readRepository, "bounded history")
	if err != nil {
		return objectmodel.HistoryScanPage{}, err
	}
	return port.ScanHistory(ctx, id, at, query)
}

func (r *datasetRepository) ScanHistory(ctx context.Context, id objectmodel.ObjectID, at kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
	if err := r.check(id, at); err != nil {
		return objectmodel.HistoryScanPage{}, err
	}
	if datasetRestrictsRepository(r.serving.pin.Items, r.ID()) {
		return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset file scope does not publish repository history")
	}
	port, ok := r.Repository.(objectmodel.HistoryScanner)
	if !ok {
		return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no bounded object history", r.ID())
	}
	return port.ScanHistory(ctx, id, at, query)
}
