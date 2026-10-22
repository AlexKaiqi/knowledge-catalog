package opensearchimpl

import (
	"bytes"
	"context"
	"slices"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

const continuationGrace = 2 * time.Minute

// Scope excludes position/rank: changing page size and advancing within the
// same logical query never creates a new lease or extends its original limit.
func continuationProtectionEvidence(evidence []byte, state pitContinuation) []byte {
	return []byte(kernel.CanonicalDigest(struct {
		Binding           kernel.Digest
		Repository        kernel.RepositoryID
		Namespace         kernel.Digest
		Basis             kernel.CommitID
		Generation, Query string
		Until             int64
	}{kernel.CanonicalDigest(evidence), state.Repository, state.Namespace, state.Basis, state.Generation, state.Query, state.Until}))
}

func (e *openSearchEngine) protectContinuation(ctx context.Context, state *pitContinuation) error {
	q := e.protectionForContext(ctx)
	if q == nil {
		return nil
	}
	if state.Lease != "" {
		return e.checkContinuationProtection(ctx, *state)
	}
	if err := q.status(); err != nil {
		return err
	}
	id, err := newResourceID()
	if err != nil {
		return err
	}
	state.Lease, state.Until = id, time.Now().Add(continuationGrace).UnixMilli()
	phase := claimContinuation
	if q.claim.Phase == claimRootQuery {
		phase = claimRootContinuation
	}
	claim := resourceClaim{ID: id, Phase: phase, Until: state.Until, Evidence: continuationProtectionEvidence(q.claim.Evidence, *state)}
	session := *q.session
	session.resourceContext = ctx
	return session.changeResource(func(resource *generationResource) error {
		if resource.Phase != resourceAdopted || resource.Pending {
			return kernel.Fail(kernel.ErrPreconditionFailed, "continuation resource changed before admission")
		}
		held := slices.ContainsFunc(resource.Claims, func(current resourceClaim) bool {
			return current.ID == q.claim.ID && current.Phase == q.claim.Phase && current.Until == q.until && current.Until > time.Now().UnixMilli() && bytes.Equal(current.Evidence, q.claim.Evidence)
		})
		if !held {
			return kernel.Fail(kernel.ErrPreconditionFailed, "query protection ended before continuation admission")
		}
		resource.Claims = append(resource.Claims, claim)
		return nil
	})
}

func continuationClaim(resource *generationResource, evidence []byte, state pitContinuation) (resourceClaim, error) {
	return continuationClaimFor(resource, evidence, state, false)
}

func continuationClaimFor(resource *generationResource, evidence []byte, state pitContinuation, root bool) (resourceClaim, error) {
	if resource == nil || resource.Version < 4 || resource.Phase != resourceAdopted || resource.Pending {
		return resourceClaim{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation resource protection is unavailable")
	}
	if err := validateResourceProtection(resource); err != nil {
		return resourceClaim{}, err
	}
	if !validGeneration(state.Lease) || state.Until <= time.Now().UnixMilli() {
		return resourceClaim{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation protection expired; restart the query")
	}
	phase := claimContinuation
	if root {
		if resource.Version < 5 || resource.QueryLeases != 1 {
			return resourceClaim{}, kernel.Fail(kernel.ErrPreconditionFailed, "ROOT continuation lacks query coverage")
		}
		phase = claimRootContinuation
	}
	expected := continuationProtectionEvidence(evidence, state)
	for _, claim := range resource.Claims {
		if claim.ID == state.Lease && claim.Phase == phase && claim.Until == state.Until && bytes.Equal(claim.Evidence, expected) {
			return claim, nil
		}
	}
	return resourceClaim{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation has no matching durable protection")
}

func (e *openSearchEngine) checkContinuationProtection(ctx context.Context, state pitContinuation) error {
	q := e.protectionForContext(ctx)
	if q == nil {
		return nil
	}
	if err := q.status(); err != nil {
		return err
	}
	if q.continuation != nil {
		anchor := q.continuation
		if state.Lease != anchor.ID || state.Until != anchor.Until || !bytes.Equal(anchor.Evidence, continuationProtectionEvidence(q.claim.Evidence, state)) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "continuation differs from the admitted query scope")
		}
		return nil // This in-flight request has its own bounded durable lease.
	}
	resource, version, err := e.loadControlRecord(ctx, q.session.resourceID())
	if err != nil {
		return err
	}
	if version == nil || resource.State != "RESOURCE" || resource.Repository != string(e.repository) || resource.Generation != q.session.generation || resource.ActiveIndex != q.session.physicalIndex {
		return kernel.Fail(kernel.ErrPreconditionFailed, "continuation resource identity is unavailable")
	}
	_, err = continuationClaimFor(resource.Resource, q.claim.Evidence, state, q.claim.Phase == claimRootQuery)
	return err
}

func (e *openSearchEngine) BindPublicationContinuation(ctx context.Context, evidence []byte, token string) (ports.Engine, error) {
	return e.bindPublicationContinuation(ctx, evidence, token, time.Time{})
}

func (e *openSearchEngine) BindPublicationContinuationUntil(ctx context.Context, evidence []byte, token string, until time.Time) (ports.Engine, error) {
	if until.IsZero() {
		return nil, expiredObservation()
	}
	// Resource deadlines and admission contexts use durable millisecond precision.
	// Normalize once so timeout classification uses the same instant as the CAS.
	until = time.UnixMilli(until.UnixMilli())
	if !until.After(time.Now()) {
		return nil, expiredObservation()
	}
	return e.bindPublicationContinuation(ctx, evidence, token, until)
}

func (e *openSearchEngine) bindPublicationContinuation(ctx context.Context, evidence []byte, token string, until time.Time) (ports.Engine, error) {
	state, err := decodePITContinuation(token)
	if err != nil || len(evidence) == 0 || !validGeneration(state.Generation) || state.Namespace != namespaceDigest(e.namespace) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid publication continuation scope")
	}
	session := &rebuildSession{engine: e, generation: state.Generation, physicalIndex: e.prefix + "-g-" + state.Generation, resourceContext: ctx}
	resource, version, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		return nil, err
	}
	if version == nil || resource.State != "RESOURCE" || resource.Repository != string(e.repository) || resource.Generation != state.Generation || resource.ActiveIndex != session.physicalIndex || resource.Basis != string(state.Basis) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication continuation resource is unavailable")
	}
	anchor, err := continuationClaim(resource.Resource, evidence, state)
	if err != nil {
		return nil, err
	}
	ticket := &preparedResource{id: anchor.ID, queryAnchor: &anchor, session: session}
	if !until.IsZero() {
		ticket.observation = &observationAdmission{record: resource, until: until.UnixMilli()}
	}
	bound, err := e.openPublicationView(ctx, resource, *version, ticket, evidence)
	if err != nil && !until.IsZero() && !until.After(time.Now()) && (ctx == nil || ctx.Err() == nil) {
		return nil, expiredObservation()
	}
	return bound, err
}
