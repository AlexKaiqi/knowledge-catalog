package projection

import (
	"context"
	"errors"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// RecoverResources runs on the maintenance path, including when all demands
// are already READY. Snapshotting handles avoids holding the runtime lock
// across provider IO. Opening/querying an engine never triggers this work.
func (idx *Index) RecoverResources(ctx context.Context) error {
	idx.mu.Lock()
	maintainers := make([]ports.ResourceMaintainer, 0, len(idx.engs))
	for _, engine := range idx.engs {
		if maintainer, ok := engine.(ports.ResourceMaintainer); ok {
			maintainers = append(maintainers, maintainer)
		}
	}
	directory := idx.dir
	coordinator, resolve := idx.resourceCoordinator, idx.publicationRecovery
	referenced := idx.publicationReference
	idx.mu.Unlock()
	joined := idx.recoverPublishedDatasets(ctx, "", 0)
	if coordinator != nil {
		maintainers = []ports.ResourceMaintainer{coordinator}
	}
	for _, maintainer := range maintainers {
		if err := ctx.Err(); err != nil {
			return errors.Join(joined, err)
		}
		if publication, ok := maintainer.(ports.PublicationResourceMaintainer); ok && resolve != nil {
			references, err := publication.RecoverPublications(ctx, resolve)
			joined = errors.Join(joined, err)
			if len(references.Retained) > 0 {
				joined = errors.Join(joined, kernel.Fail(kernel.ErrTemporaryUnavailable, "publication resources retain unknown acceptance: %v", references.Retained))
			}
		}
		if references, ok := maintainer.(ports.PublicationReferenceMaintainer); ok && referenced != nil {
			report, err := references.ReconcilePublicationReferences(ctx, referenced)
			joined = errors.Join(joined, err)
			if len(report.Retained) > 0 {
				joined = errors.Join(joined, kernel.Fail(kernel.ErrTemporaryUnavailable, "accepted references retain unknown eligibility: %v", report.Retained))
			}
		}
		if computations, ok := maintainer.(ports.ComputationMaintainer); ok {
			report, err := computations.RecoverComputations(ctx, directory)
			joined = errors.Join(joined, err)
			if len(report.Retained) > 0 {
				joined = errors.Join(joined, kernel.Fail(kernel.ErrTemporaryUnavailable, "computation resources retain unknown ownership: %v", report.Retained))
			}
		}
		report, err := maintainer.RecoverResources(ctx)
		joined = errors.Join(joined, err)
		if len(report.Retained) > 0 {
			joined = errors.Join(joined, kernel.Fail(kernel.ErrTemporaryUnavailable, "physical resources retain unresolved protection: %v", report.Retained))
		}
	}
	return joined
}

// SetResourceRecovery connects the instance authority independently of query
// handles. Resolve is supplied by the composition root owning Dataset access.
// It must establish an authority fence after receiving each durable admission.
func (idx *Index) SetResourceRecovery(coordinator ports.ResourceCoordinator, resolve func(context.Context, []byte) (bool, error)) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.resourceCoordinator, idx.publicationRecovery = coordinator, resolve
}

// SetPublishedDatasetRecovery supplies an explicit accepted-Dataset maintenance
// pass. Empty setID/revision zero requests instance inventory; a named pair
// requests only that accepted release before explicit preparation. The callback
// runs without the Index lock, independently of opened query scopes.
func (idx *Index) SetPublishedDatasetRecovery(recover func(context.Context, string, int) error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.publishedDatasetRecovery = recover
}

// SetPublicationReferenceLookup supplies the separate live-consumer authority.
// Acceptance receipts alone cannot decide that a resource is still referenced.
func (idx *Index) SetPublicationReferenceLookup(referenced func(context.Context, []byte) (bool, error)) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.publicationReference = referenced
}

// Serialize accepted-Dataset repairs with explicit preparation in this instance.
// The actual authority CAS still rejects intervening writes by other processes.
func (idx *Index) recoverPublishedDatasets(ctx context.Context, setID string, revision int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	idx.mu.Lock()
	recover := idx.publishedDatasetRecovery
	if idx.publishedRecoveryGate == nil {
		idx.publishedRecoveryGate = make(chan struct{}, 1)
	}
	gate := idx.publishedRecoveryGate
	idx.mu.Unlock()
	if recover == nil {
		return nil
	}
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-gate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return recover(ctx, setID, revision)
}
