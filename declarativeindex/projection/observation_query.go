package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

// acquireObservation fixes the provider generation and matching immutable
// values together. stateMu keeps borrowed file handles alive for this request;
// an older generation's independently opened handle is closed on release.
func (idx *Index) acquireObservation(ctx context.Context, key engineKey, authority kernel.RepositoryID, request retrieval.SearchRequest, dataset *datasetSpace) (ports.Engine, *stateProjection, func(), error) {
	idx.stateMu.RLock()
	resident := idx.states[key]
	mismatch := resident != nil && resident.authority != authority
	idx.stateMu.RUnlock()
	if mismatch {
		return nil, nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "resident observation belongs to another source binding")
	}
	owner, err := idx.stateEngineAt(authority, key.commit)
	if err != nil {
		return nil, nil, nil, err
	}
	binder, managed := owner.(ports.ObservationBinder)
	_, resourceManaged := owner.(ports.ResourcePreparer)
	accepted := dataset != nil && !dataset.pin.Temporary && idx.observationEvidence != nil && resourceManaged
	if !managed && !accepted {
		if err := idx.restoreState(ctx, key, authority); err != nil {
			return nil, nil, nil, err
		}
		idx.stateMu.RLock()
		state := idx.states[key]
		return owner, state, idx.stateMu.RUnlock, nil
	}
	var bound ports.Engine
	if accepted {
		bound, err = idx.bindDatasetObservation(ctx, owner, dataset.pin.SetID, dataset.pin.Revision, request)
	} else {
		position := ""
		if request.Continuation != "" {
			cursor, decodeErr := retrieval.DecodeContinuation(request.Continuation)
			if decodeErr != nil || cursor.Scope != "repository" || cursor.Position == "" {
				return nil, nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid observation search continuation")
			}
			position = cursor.Position
		}
		bound, err = binder.BindObservation(ctx, position)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if bound == nil {
		return nil, nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider returned no fixed observation view")
	}
	idx.stateMu.RLock()
	fail := func(err error) (ports.Engine, *stateProjection, func(), error) {
		idx.stateMu.RUnlock()
		_ = bound.Close()
		return nil, nil, nil, err
	}
	meta, err := loadMetaContext(ctx, bound)
	if err != nil {
		return fail(err)
	}
	state := idx.states[key]
	borrowed := state != nil && state.authority == authority && idx.stateProjectionMatches(bound, meta, key.commit, state)
	if !borrowed {
		state, err = idx.openObservationState(ctx, key, authority, bound, meta)
		if err != nil {
			return fail(err)
		}
	}
	if state == nil {
		return fail(kernel.Fail(kernel.ErrPreconditionFailed, "fixed observation values are unavailable"))
	}
	release := func() {
		if !borrowed {
			_ = state.store.Close()
		}
		idx.stateMu.RUnlock()
		_ = bound.Close()
	}
	return bound, state, release, nil
}
