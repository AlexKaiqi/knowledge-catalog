package opensearchimpl

import (
	"context"
	"reflect"

	ports "kc/discovery/projection"
	"kc/kernel"
)

type rootQueryContextKey struct{}

// The context belongs to one concrete owner, not other engines that happen to
// receive the same caller context. No shared engine field is changed per query.
func (e *openSearchEngine) protectionForContext(ctx context.Context) *queryProtection {
	if e.queryProtection != nil {
		return e.queryProtection
	}
	if ctx != nil {
		if protection, ok := ctx.Value(rootQueryContextKey{}).(*queryProtection); ok && protection.session.engine == e {
			return protection
		}
	}
	return nil
}

func rootQueryEvidence(record controlDoc) []byte {
	meta := readyResourceMeta(record)
	meta.Mode, meta.Cause, meta.State = "", "", ""
	meta.Coverage = 0
	return []byte(kernel.CanonicalDigest(struct {
		Domain, Repository string
		Meta               ports.Meta
	}{"root-query-v1", record.Repository, meta}))
}

// Fresh resources close the untracked ROOT-reader path before they become
// reclaimable. Legacy resources never acquire this guarantee by codec promotion.
func (e *openSearchEngine) protectRootQuery(ctx context.Context, root controlDoc, version *controlVersion, token string) (context.Context, func(), error) {
	if e.protectionForContext(ctx) != nil || root.QueryLeaseVersion == 0 {
		return ctx, func() {}, nil
	}
	if root.QueryLeaseVersion != 1 || root.State != ports.ProjectionStateReady || version == nil || root.Repository != string(e.repository) || !validGeneration(root.Generation) || root.ActiveIndex != e.prefix+"-g-"+root.Generation {
		return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "ROOT query protection identity is unavailable")
	}
	session := &rebuildSession{engine: e, generation: root.Generation, physicalIndex: root.ActiveIndex, resourceContext: ctx}
	resource, stored, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		return nil, nil, err
	}
	if stored == nil || resource.Resource == nil || resource.Resource.QueryLeases != 1 || resource.Repository != root.Repository || resource.ActiveIndex != root.ActiveIndex || resource.Generation != root.Generation || !reflect.DeepEqual(readyResourceMeta(resource), readyResourceMeta(root)) {
		return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "ROOT query lacks its exact managed resource")
	}
	evidence := rootQueryEvidence(root)
	ticket := &preparedResource{session: session, rootQuery: &root}
	if token != "" {
		state, err := decodePITContinuation(token)
		if err != nil || state.Namespace != namespaceDigest(e.namespace) || state.Generation != root.Generation {
			return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "ROOT continuation differs from current projection")
		}
		anchor, err := continuationClaimFor(resource.Resource, evidence, state, true)
		if err != nil {
			return nil, nil, err
		}
		ticket.queryAnchor = &anchor
	}
	protection, err := ticket.protectQuery(ctx, evidence)
	if err != nil {
		return nil, nil, err
	}
	current, actual, err := e.loadControlContext(ctx)
	if err == nil && (actual == nil || *actual != *version || !reflect.DeepEqual(current, root)) {
		err = kernel.Fail(kernel.ErrPreconditionFailed, "ROOT changed while admitting its query")
	}
	if err != nil {
		_ = protection.close()
		return nil, nil, err
	}
	return context.WithValue(ctx, rootQueryContextKey{}, protection), func() { _ = protection.close() }, nil
}
