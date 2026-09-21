package cli

import (
	"context"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/knowledgeapp"
	"kc/retrieval"
)

func searchWorkspace(cx *invocation) (any, error) {
	req, err := searchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.DatasetSearchExecutor{
		Authorize: func(context.Context) error { return authorize(cx.Home, "knowledge.search", cx.Flags, nil, cx.WS) },
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			serving, _, err := openServingAt(cx, cx.WS, cx.Flags)
			if err != nil {
				return nil, nil, err
			}
			logical, err := logicalWorkspaceServing(cx, serving)
			return serving, logical, err
		},
		Repositories: cx.WS.Reader, Projection: cx.WS.Index,
		Deliver: func(_ context.Context, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
			return deliverSearchHit(cx.Home, cx.Flags, hit)
		},
	}).Execute(cx.Context, req)
}
func logicalWorkspaceServing(cx *invocation, declarations *reader.Serving) (*knowledgeserving.Service, error) {
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return knowledgeserving.OpenRequest(declarations, cx.State, request), nil
}

func stateRequestContextFrom(cx *invocation) (knowledgeserving.RequestContext, error) {
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	trace, err := traceContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	requestID, err := requestIDFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	return knowledgeserving.RequestContext{
		Identity: identity, Trace: trace, RequestID: requestID,
	}, nil
}

func hydrateSearchHit(ctx context.Context, logical *knowledgeserving.Service, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	return knowledgeapp.HydrateSearchHit(ctx, logical, hit)
}
