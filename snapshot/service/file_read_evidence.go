package service

import (
	"context"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/dataset"
)

// readFile records the final request outcome once, before successful bytes can
// leave the service. Source readers supply coordinates, not separate receipts.
func (resources Resources) readFile(ctx context.Context, caller identity.HTTPIdentity, request FileReadRequest) (result any, callErr error) {
	record := resources.RecordRead
	evidence := requestedFileEvidence(request)
	resources.RecordRead = func(_ context.Context, _ identity.HTTPIdentity, actual FileReadEvidence) error {
		evidence = actual
		return nil
	}
	if request.Repository != "" || request.Commit != "" {
		result, callErr = resources.readRepositoryFile(ctx, caller, request)
	} else {
		var view FileView
		view, callErr = resources.OpenFiles(ctx, caller, request.FileCoordinate, true)
		if callErr == nil {
			// Only the reconstructed, accepted pin is trusted, including on read errors.
			evidence.Pin = view.FixedPin()
			result, callErr = view.Read(request)
		}
	}
	if record != nil {
		evidence.Err = callErr
		if err := record(ctx, caller, evidence); err != nil && callErr == nil {
			return nil, err
		}
	}
	return result, callErr
}

func requestedFileEvidence(request FileReadRequest) FileReadEvidence {
	evidence := FileReadEvidence{Pin: dataset.ResolvedDataset{SetID: request.Dataset}}
	// A denied native request may retain its explicit attempted target without
	// inspecting a source. Never derive target coordinates from a caller pin.
	if request.Repository != "" && request.Commit != "" && ValidRelativePath(request.Path) &&
		request.Dataset == "" && len(request.Pin) == 0 && request.View == "" && request.MountPath == "" && request.File == "" {
		evidence.Repository = kernel.RepositoryID(request.Repository)
		evidence.Commit = kernel.CommitID(request.Commit)
		evidence.Path = request.Path
	}
	return evidence
}
