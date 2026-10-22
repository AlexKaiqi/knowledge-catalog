package datasets

import (
	"context"
	"errors"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// Adopt prepares a legacy immutable release and attaches its unique receipt.
// A nil protection means this provider has no managed resources and leaves the
// receipt-free release unchanged. Unknown admission or commit outcomes retain
// the external protection for normal authority recovery.
func (r PublicationRecovery) Adopt(ctx context.Context, key dataset.PublicationLookup, prepare func(context.Context, dataset.Dataset) (PublicationProtection, error)) (err error) {
	adopter, ok := r.Authority.(dataset.PublicationAdopter)
	if !ok || prepare == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "publication preparation adoption is unavailable")
	}
	var ticket PublicationProtection
	attempted := false
	unmanaged := errors.New("provider has no managed preparation")
	defer func() {
		if ticket != nil {
			if !attempted {
				err = errors.Join(err, ticket.Cancel(ctx))
			}
			err = errors.Join(err, ticket.Close())
		}
	}()
	_, err = adopter.AdoptPublication(ctx, key, func(ctx context.Context, definition dataset.Dataset, admission dataset.PublicationAdmission) error {
		identity := kernel.CanonicalDigest(definition)
		var err error
		ticket, err = prepare(ctx, definition)
		if err != nil {
			return err
		}
		if kernel.CanonicalDigest(definition) != identity {
			return kernel.Fail(kernel.ErrPreconditionFailed, "preparation adoption changed immutable release")
		}
		if ticket == nil {
			return unmanaged
		}
		evidence, err := marshalPublicationEvidence(definition, admission)
		if err != nil {
			return err
		}
		attempted = true
		return ticket.Admit(ctx, evidence)
	})
	if errors.Is(err, unmanaged) {
		return nil
	}
	if err == nil && ticket != nil {
		return ticket.Confirm(ctx)
	}
	return err
}
