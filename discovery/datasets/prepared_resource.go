package datasets

import (
	"context"
	"encoding/json"
	"errors"

	"kc/snapshot/dataset"
)

// PublicationProtection is the publication executor's durable preparation
// handle. Admission errors are uncertain: Cancel is permitted only before any
// admission attempt. Close releases a local borrow without releasing protection.
// Physical providers implement their own resource port; the composition root
// supplies this consumer view without exposing a provider dependency here.
type PublicationProtection interface {
	Admit(context.Context, []byte) error
	Confirm(context.Context) error
	Cancel(context.Context) error
	Close() error
}

// The full accepted candidate is retained with the authority coordinates, so
// recovery can match its exact intent against immutable authority state. This
// evidence belongs to a publication claim, not to a source membership list.
type publicationResourceEvidence struct {
	Version    int                          `json:"version"`
	Admission  string                       `json:"admission,omitempty"`
	Authority  dataset.PublicationAuthority `json:"authority"`
	Definition dataset.Dataset              `json:"definition"`
}

func (e DatasetPublisher) publishPrepared(ctx context.Context, def dataset.Dataset, ticket PublicationProtection) (result dataset.Dataset, err error) {
	if ticket == nil {
		return e.Registry.PublishDataset(def)
	}
	attempted := false
	defer func() {
		if !attempted {
			err = errors.Join(err, ticket.Cancel(ctx))
		}
		err = errors.Join(err, ticket.Close())
	}()
	coordinator := e.Registry.(dataset.PublicationCoordinator) // Checked before allocation.
	result, err = coordinator.PublishDatasetGuarded(ctx, def, func(ctx context.Context, authority dataset.PublicationAdmission) error {
		evidence, err := marshalPublicationEvidence(def, authority)
		if err != nil {
			return err
		}
		// Even a failing admission may have been durably accepted before its
		// response was lost. Only authority recovery may release it thereafter.
		attempted = true
		return ticket.Admit(ctx, evidence)
	})
	if err == nil {
		err = ticket.Confirm(ctx)
	}
	return result, err
}

func (p Preparation) ExecuteProtected(ctx context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
	if p.Enabled && p.PrepareProtectedDataset != nil {
		prepared, err := p.prepareDefinition(def)
		if err != nil {
			return dataset.Dataset{}, nil, err
		}
		ticket, err := p.PrepareProtectedDataset(ctx, dataset.ResolvedDataset{SetID: prepared.SetID, Revision: prepared.Revision, Repositories: frozenRepositories(prepared), Items: prepared.Items, Dependencies: prepared.Dependencies})
		return prepared, ticket, err
	}
	prepared, err := p.Execute(ctx, def)
	if err != nil || !p.Enabled || p.ProtectDataset == nil {
		return prepared, nil, err
	}
	ticket, err := p.ProtectDataset(ctx, dataset.ResolvedDataset{SetID: prepared.SetID, Revision: prepared.Revision,
		Repositories: frozenRepositories(prepared), Items: prepared.Items, Dependencies: prepared.Dependencies})
	return prepared, ticket, err
}

func marshalPublicationEvidence(def dataset.Dataset, authority dataset.PublicationAdmission) ([]byte, error) {
	return json.Marshal(publicationResourceEvidence{Version: 2, Authority: authority.PublicationAuthority, Admission: authority.ID, Definition: def})
}
