package projection

import (
	"context"

	ports "kc/discovery/projection"
	"kc/kernel"
)

// SetDatasetPublicationLookup supplies accepted, opaque preparation evidence.
// It is independent of source reading basis and grants no query authority.
func (idx *Index) SetDatasetPublicationLookup(lookup func(context.Context, string, int) ([]byte, error)) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.publicationEvidence = lookup
}

func (idx *Index) acquireDatasetPublication(ctx context.Context, s datasetSpace, continuation datasetContinuation) (ports.Engine, func(), error) {
	idx.mu.Lock()
	lookup := idx.publicationEvidence
	cached := idx.engs[engineKey{repo: s.id, commit: s.basis}]
	idx.mu.Unlock()
	// Open the exact fixed scope locally, without reading or falling back through
	// the mutable projection pointer before binding the accepted publication.
	owner := cached
	closeOwner := func() {}
	if owner == nil {
		var err error
		owner, err = idx.open(idx.dir, pinOpenID(s.id, s.basis))
		if err != nil {
			return nil, nil, err
		}
		closeOwner = func() { _ = owner.Close() }
	}
	if lookup == nil || s.pin.Temporary {
		return owner, closeOwner, nil
	}
	binder, managed := owner.(ports.PublicationBinder)
	if !managed {
		return owner, closeOwner, nil
	}
	evidence, err := lookup(ctx, s.pin.SetID, s.pin.Revision)
	if err != nil {
		closeOwner()
		return nil, nil, err
	}
	if len(evidence) == 0 {
		return owner, closeOwner, nil
	}
	position, err := continuation.position(ctx, owner)
	if err != nil {
		closeOwner()
		return nil, nil, err
	}
	var bound ports.Engine
	if position != "" {
		fixed, ok := owner.(ports.PublicationContinuationBinder)
		if !ok {
			closeOwner()
			return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider cannot resume protected publication continuation")
		}
		bound, err = fixed.BindPublicationContinuation(ctx, evidence, position)
	} else {
		bound, err = binder.BindPublication(ctx, evidence)
	}
	if err != nil {
		closeOwner()
		return nil, nil, err
	}
	if bound == nil {
		closeOwner()
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "accepted publication returned no query view")
	}
	return bound, func() { _ = bound.Close(); closeOwner() }, nil
}

// The outer relation cursor must be authenticated before choosing its original
// provider position; the complete logical coordinates are rechecked at execution.
type datasetContinuation struct {
	positionToken string
	relationToken string
}

func (c datasetContinuation) position(ctx context.Context, owner ports.Engine) (string, error) {
	if c.relationToken == "" {
		return c.positionToken, nil
	}
	if c.positionToken != "" {
		return "", invalidRelationContinuation()
	}
	sealer, err := relationSealer(ctx, owner, true)
	if err != nil {
		return "", err
	}
	state, err := openRelationContinuation(sealer, c.relationToken)
	if err != nil {
		return "", err
	}
	return state.Position, nil
}
