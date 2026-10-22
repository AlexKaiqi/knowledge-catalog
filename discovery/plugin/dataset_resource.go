package plugin

import (
	"context"
	"sync"

	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

type datasetResource struct {
	ports.PreparedResource
	once sync.Once
	done func()
	err  error
}

func (r *datasetResource) Close() error {
	r.once.Do(func() {
		r.err = r.PreparedResource.Close()
		r.done()
	})
	return r.err
}

// ProtectDatasetScope obtains protection from the selected instance and keeps
// that instance borrowed until the ticket's local handle is closed. Closing the
// handle never releases durable publication protection.
func (p *IndexPlugins) ProtectDatasetScope(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.PreparedResource, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return nil, err
	}
	provider, ok := instance.services.Queries.(interface {
		ProtectDatasetResource(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.PreparedResource, error)
	})
	if !ok {
		done()
		return nil, nil
	}
	ticket, err := provider.ProtectDatasetResource(ctx, lookup, pin)
	if ticket == nil {
		done()
		return nil, err
	}
	return &datasetResource{PreparedResource: ticket, done: done}, err
}

// PrepareProtectedDatasetScope retains one selected instance throughout build
// and durable ticket handoff. Legacy unmanaged providers keep normal preparation.
func (p *IndexPlugins) PrepareProtectedDatasetScope(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.PreparedResource, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return nil, err
	}
	provider, ok := instance.services.Queries.(interface {
		PrepareDatasetResource(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.PreparedResource, error)
	})
	if ok {
		ticket, err := provider.PrepareDatasetResource(ctx, lookup, pin)
		if ticket == nil {
			done()
			return nil, err
		}
		return &datasetResource{PreparedResource: ticket, done: done}, err
	}
	_, managed := instance.services.Queries.(interface {
		ProtectDatasetResource(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.PreparedResource, error)
	})
	done()
	if managed {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed Dataset provider requires continuous preparation protection")
	}
	return nil, p.PrepareDatasetScope(ctx, lookup, pin)
}
