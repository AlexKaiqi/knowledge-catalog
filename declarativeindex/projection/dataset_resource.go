package projection

import (
	"context"

	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

// ProtectDatasetResource pins the already prepared static corpus. It never
// builds or chooses a moving source. Providers without managed resources return
// no ticket; managed providers must prove the exact ready generation's owner.
func (idx *Index) ProtectDatasetResource(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.PreparedResource, error) {
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return nil, err
	}
	engine, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return nil, err
	}
	provider, ok := engine.(ports.ResourcePreparer)
	if !ok {
		return nil, nil
	}
	meta, err := loadMetaContext(ctx, engine)
	if err != nil {
		return nil, err
	}
	if meta.Basis != s.basis || meta.State != ports.ProjectionStateReady {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset static corpus is not ready at its frozen basis")
	}
	if _, err := idx.validateDatasetInterpretation(s, engine, meta); err != nil {
		return nil, err
	}
	ticket, err := provider.PrepareResource(ctx, meta)
	if err == nil && ticket == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed Dataset preparation requires a resource protection ticket")
	}
	return ticket, err
}

// PrepareLegacyDatasetResource rebuilds from the original fixed scope before
// attaching a first durable preparation receipt. Existing resources without
// proven ownership are never adopted by name or overwritten in place.
func (idx *Index) PrepareLegacyDatasetResource(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.PreparedResource, error) {
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return nil, err
	}
	engine, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return nil, err
	}
	if _, ok := engine.(ports.ResourcePreparer); !ok {
		return nil, nil
	}
	_, ticket, err := idx.prepareDatasetProtected(ctx, lookup, pin, true, true)
	return ticket, err
}

func buildDatasetResource(engine ports.Engine, docs []ports.CompiledDoc, meta ports.Meta) (ports.PreparedResource, error) {
	streaming, ok := engine.(ports.StreamingProjectionMaintainer)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed Dataset build requires protected streaming preparation")
	}
	session, err := streaming.BeginRebuild(meta)
	if err != nil {
		return nil, err
	}
	protected, ok := session.(ports.PreparedRebuildSession)
	if !ok {
		return nil, session.Abort(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed Dataset build requires continuous resource protection"))
	}
	if err := session.Append(docs); err != nil {
		return nil, session.Abort(err)
	}
	ticket, err := protected.CommitResource()
	if err == nil && ticket == nil {
		err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed Dataset build returned no resource ticket")
	}
	return ticket, err
}
