package dataset

import (
	"context"

	"kc/kernel"
)

// PublicationAdopter attaches preparation to an existing immutable release.
// The callback may protect external resources, but cannot replace the release.
// Acceptance creates only a receipt and never a new Dataset revision.
type PublicationAdopter interface {
	AdoptPublication(context.Context, PublicationLookup, func(context.Context, Dataset, PublicationAdmission) error) (Dataset, error)
}

func (c *service) AdoptPublication(ctx context.Context, lookup PublicationLookup, admit func(context.Context, Dataset, PublicationAdmission) error) (Dataset, error) {
	if admit == nil {
		return Dataset{}, kernel.Fail(kernel.ErrUsageInvalid, "preparation adoption requires admission")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Refresh from the actual authority and exclude publishers at earlier bases.
	checkpoint, err := c.FencePublications(ctx, lookup)
	if err != nil {
		return Dataset{}, err
	}
	definition, err := c.DatasetVersion(lookup.SetID, lookup.Revision)
	if err != nil {
		return Dataset{}, err
	}
	digest := kernel.CanonicalDigest(definition)
	for _, receipt := range checkpoint.Receipts {
		if receipt.PublicationLookup == lookup {
			if receipt.Definition != digest {
				return Dataset{}, kernel.Fail(kernel.ErrPreconditionFailed, "existing preparation receipt differs from immutable release")
			}
			return definition, nil
		}
	}
	if _, err := c.ResolveDefinition(definition); err != nil {
		return Dataset{}, err
	}
	admission, err := c.registry.publicationAdmission(string(checkpoint.Authority.Head))
	if err != nil {
		return Dataset{}, err
	}
	receipt := &PublicationReceipt{Version: 1, PublicationLookup: lookup, Admission: admission.ID, Definition: digest, Authority: admission.PublicationAuthority}
	if err := c.registry.preparePublicationReceipt(ctx, *receipt); err != nil {
		return Dataset{}, err
	}
	// The callback is outside commit locks and receives an independent value.
	if err := admit(ctx, cloneDataset(definition), admission); err != nil {
		return Dataset{}, err
	}
	err = c.commitDatasetReceiptContext(ctx, lookup.SetID, "dataset-adopt-preparation "+lookup.SetID, receipt, func(state *acceptedState) (State, error) {
		if err := c.checkAdmittedAuthority(state.head, admission.PublicationAuthority); err != nil {
			return State{}, err
		}

		current, ok := state.datasets[lookup.SetID]
		if !ok || current.Retired {
			return State{}, kernel.Fail(kernel.ErrDatasetInvalid, "preparation adoption target is absent or retired")
		}
		fixed, ok := state.versions[lookup.SetID][lookup.Revision]
		if !ok || kernel.CanonicalDigest(fixed) != digest {
			return State{}, kernel.Fail(kernel.ErrPreconditionFailed, "preparation adoption changed immutable release")
		}
		return state.dump(c.registry.AuthorityID()), nil
	})
	if err != nil {
		return Dataset{}, err
	}
	return definition, nil
}
