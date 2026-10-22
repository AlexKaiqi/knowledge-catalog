package opensearchimpl

import (
	"context"
	"errors"
	"slices"

	"kc/kernel"
)

// Managed generations use the same CAS exclusion for incremental writes and
// publication claims. Legacy generations remain unclaimable; their existing
// update path does not confer ownership or permit resource reclamation.
func (e *openSearchEngine) updateResource(control controlDoc, update func(func(func() error) error) error) error {
	ctx := context.Background()
	session := &rebuildSession{engine: e, physicalIndex: control.ActiveIndex, generation: control.Generation, resourceContext: ctx}
	_, version, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		return err
	}
	if version == nil {
		return update(nil)
	}
	if err := session.changeResource(func(resource *generationResource) error {
		prunePreparedClaims(resource)
		if resource.Phase != resourceAdopted || resource.Pending || slices.ContainsFunc(resource.Claims, func(claim resourceClaim) bool { return claim.Phase != claimRootContinuation }) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "projection is protected by a resource claim or unresolved write")
		}
		resource.Pending = true
		return nil
	}); err != nil {
		return err
	}
	guard := func(operation func() error) error {
		if err := session.changeResource(func(resource *generationResource) error {
			if resource.Phase != resourceAdopted || !resource.Pending {
				return kernel.Fail(kernel.ErrPreconditionFailed, "incremental resource admission closed")
			}
			return nil
		}); err != nil {
			return err
		}
		return operation()
	}
	if err := update(guard); err != nil {
		var completed *completedPhysicalError
		if errors.As(err, &completed) {
			clear := session.changeResource(func(resource *generationResource) error {
				if resource.Phase != resourceAdopted || !resource.Pending {
					return kernel.Fail(kernel.ErrPreconditionFailed, "rejected update lost its admission")
				}
				resource.Pending = false
				return nil
			})
			return errors.Join(err, clear)
		}
		return err // Unknown physical/control completion retains write exclusion.
	}
	ready, _, err := e.loadControlContext(ctx)
	if err != nil {
		return err
	}
	if ready.Repository != string(e.repository) || ready.State != "READY" || ready.Generation != control.Generation || ready.ActiveIndex != control.ActiveIndex {
		return kernel.Fail(kernel.ErrPreconditionFailed, "updated resource no longer has confirmed ready metadata")
	}
	return session.changeResourceRecord(func(record *controlDoc) error {
		resource := record.Resource
		if resource.Phase != resourceAdopted || !resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resource write admission changed")
		}
		resource.Pending = false
		ready.State, ready.Resource = "RESOURCE", resource
		ready.ObservationDeadlines = nil
		*record = ready
		return nil
	})
}
