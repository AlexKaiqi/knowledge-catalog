package opensearchimpl

import (
	"context"
	"errors"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func (e *openSearchEngine) RepairPublication(ctx context.Context, evidence []byte, desired ports.Meta, prepare func(context.Context) (ports.Meta, error), accept func(func() error) error) error {
	if err := e.requireMutableProjection(); err != nil {
		return err
	}
	if len(evidence) == 0 || prepare == nil || accept == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "publication repair requires evidence, preparation and authority acceptance")
	}
	id := e.publicationBindingID(evidence)
	original, version, err := e.loadControlRecord(ctx, id)
	if err != nil {
		return err
	}
	if version != nil {
		if !validGeneration(original.Generation) || !validGeneration(original.PublicationTicket) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "publication repair cannot replace unknown binding identity")
		}
		ticket := &preparedResource{id: original.PublicationTicket, session: &rebuildSession{engine: e, generation: original.Generation, physicalIndex: e.prefix + "-g-" + original.Generation}}
		if err := ticket.checkPublicationBinding(original, evidence); err != nil {
			return err
		}
		view, err := e.BindPublication(ctx, evidence)
		if err == nil {
			actual, readErr := loadEngineMeta(ctx, view)
			closeErr := view.Close()
			if readErr != nil || closeErr != nil {
				return errors.Join(readErr, closeErr)
			}
			if publicationMetaCompatible(actual, desired) {
				return nil
			}
		} else {
			var missing *missingPublicationResource
			if !errors.As(err, &missing) {
				return err
			}
		}
	}
	meta, err := prepare(ctx)
	if err != nil {
		return err
	}
	if !publicationMetaCompatible(meta, desired) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication repair does not satisfy the current fixed interpretation")
	}
	prepared, err := e.PrepareResource(ctx, meta)
	if err != nil {
		return err
	}
	ticket := prepared.(*preparedResource)
	attempted := false
	defer ticket.Close()
	if err := accept(func() error {
		attempted = true
		if err := ticket.admitClaim(ctx, evidence); err != nil {
			return err
		}
		replacement := ticket.ready
		replacement.State = publicationBindingState
		replacement.PublicationVersion, replacement.PublicationTicket, replacement.PublicationEvidence = 1, ticket.id, append([]byte(nil), evidence...)
		_, err := e.putControlRecordContext(ctx, id, replacement, version)
		if err == nil {
			return nil
		}
		// An absent acknowledgement cannot undo admission. Prove the exact
		// replacement from medium before treating the repair as accepted.
		actual, _, readErr := e.loadControlRecord(ctx, id)
		if readErr == nil && ticket.checkPublicationBinding(actual, evidence) == nil {
			return nil
		}
		return errors.Join(err, readErr)
	}); err != nil {
		if !attempted {
			return errors.Join(err, ticket.Cancel(ctx))
		}
		return err // A rejected/lost map CAS still needs durable reference recovery.
	}
	return ticket.Confirm(ctx)
}

func publicationMetaCompatible(actual, desired ports.Meta) bool {
	return (desired.Basis == "" || desired.Basis == actual.Basis) &&
		(desired.AccessDigest == "" || desired.AccessDigest == actual.AccessDigest) &&
		(desired.PhysicalDigest == "" || desired.PhysicalDigest == actual.PhysicalDigest) &&
		(desired.ProviderRevision == "" || desired.ProviderRevision == actual.ProviderRevision)
}
func loadEngineMeta(ctx context.Context, engine ports.Engine) (ports.Meta, error) {
	if loader, ok := engine.(ports.ContextMetaLoader); ok {
		return loader.LoadMetaContext(ctx)
	}
	return engine.LoadMeta()
}
