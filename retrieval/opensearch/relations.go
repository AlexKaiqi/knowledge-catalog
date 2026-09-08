package opensearch

import (
	"context"
	"fmt"
	"net/http"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func (e *openSearchEngine) RetrieveRelations(req retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	return e.RetrieveRelationsContext(context.Background(), req)
}

func (e *openSearchEngine) RetrieveRelationsContext(ctx context.Context, req retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	if err := e.readLockContext(ctx); err != nil {
		return retrieval.RelationCandidatePage{}, err
	}
	defer e.mu.RUnlock()
	if req.Repository == "" || req.Basis == "" || req.Query.Endpoint.Repository == "" || req.Query.Endpoint.Object == "" {
		return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup requires repository, basis, and endpoint KnowledgeRef")
	}
	if req.Query.Endpoint.Repository != req.Repository {
		return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation endpoint repository must equal the queried repository")
	}
	size := req.Limit
	if size <= 0 {
		size = 500
	}
	control, version, err := e.currentProjectionContext(ctx, req.Basis, req.Continuation != "")
	if err != nil {
		return retrieval.RelationCandidatePage{}, err
	}
	state := pitContinuation{}
	if req.Continuation == "" {
		state = pitContinuation{
			Basis: req.Basis, Repository: req.Repository,
			Query: string(retrieval.RelationQueryDigest(req.Query)), Generation: control.Generation,
		}
	} else {
		decoded, err := decodePITContinuation(req.Continuation)
		if err != nil {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid OpenSearch relation continuation")
		}
		state = decoded
		if state.Basis != req.Basis || state.Repository != req.Repository || state.Query != string(retrieval.RelationQueryDigest(req.Query)) || state.Generation != control.Generation {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch relation continuation does not match repository, basis, query, or generation")
		}
	}
	pit, err := e.openStablePITContext(ctx, control, version)
	if err != nil {
		return retrieval.RelationCandidatePage{}, err
	}
	state.PIT = pit
	ids, sortValues, nextPIT, err := e.searchRelationsPITContext(ctx, state, req, size)
	if err != nil {
		e.closePITContext(ctx, state.PIT)
		return retrieval.RelationCandidatePage{}, err
	}
	if nextPIT != "" {
		state.PIT = nextPIT
	}
	e.closePITContext(ctx, state.PIT)
	page := retrieval.RelationCandidatePage{Exhausted: len(ids) < size}
	for i, id := range ids {
		page.Candidates = append(page.Candidates, retrieval.RelationCandidate{
			Repository: req.Repository, ObjectID: id, Basis: state.Basis,
			Evidence: []retrieval.LaneEvidence{{Provider: e.ProviderID(), Lane: "relation", Guarantee: string(index.GuaranteeExact), LocalRank: state.Rank + i + 1}},
		})
	}
	if page.Exhausted || len(sortValues) == 0 {
		return page, nil
	}
	state.Sort = sortValues
	state.Rank += len(ids)
	state.PIT = ""
	page.Continuation = encodePITContinuation(state)
	return page, nil
}

func (e *openSearchEngine) searchRelationsPIT(state pitContinuation, req retrieval.RelationRetrieveRequest, size int) ([]knowledge.ObjectID, []any, string, error) {
	return e.searchRelationsPITContext(context.Background(), state, req, size)
}

func (e *openSearchEngine) searchRelationsPITContext(ctx context.Context, state pitContinuation, req retrieval.RelationRetrieveRequest, size int) ([]knowledge.ObjectID, []any, string, error) {
	endpointMust := []map[string]any{
		{"term": map[string]any{"relation_endpoints.repository": string(req.Query.Endpoint.Repository)}},
		{"term": map[string]any{"relation_endpoints.object_id": string(req.Query.Endpoint.Object)}},
	}
	if req.Query.Role != "" {
		endpointMust = append(endpointMust, map[string]any{"term": map[string]any{"relation_endpoints.role": req.Query.Role}})
	}
	filters := []map[string]any{
		{"term": map[string]any{"kind": string(knowledge.KindRelation)}},
		{"nested": map[string]any{
			"path":  "relation_endpoints",
			"query": map[string]any{"bool": map[string]any{"must": endpointMust}},
		}},
	}
	if req.Query.RelationType != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"relation_type": req.Query.RelationType}})
	}
	if req.Query.Direction != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"relation_direction": string(req.Query.Direction)}})
	}
	payload := map[string]any{
		"size": size, "_source": []string{"object_id"}, "track_total_hits": false,
		"pit":   map[string]any{"id": state.PIT, "keep_alive": "2m"},
		"query": map[string]any{"bool": map[string]any{"filter": filters}},
		"sort":  []any{map[string]any{"object_id": map[string]any{"order": "asc"}}},
	}
	if len(state.Sort) > 0 {
		payload["search_after"] = state.Sort
	}
	status, body, err := e.doContext(ctx, http.MethodPost, "/_search?allow_partial_search_results=false", payload)
	if err != nil {
		return nil, nil, "", err
	}
	if status >= 400 {
		return nil, nil, "", fmt.Errorf("opensearch relation search: %s", body)
	}
	ids, sorts, pit, err := decodeSearchResponse(body, 1)
	if err != nil {
		return nil, nil, "", err
	}
	var lastSort []any
	if len(sorts) > 0 {
		lastSort = sorts[len(sorts)-1]
	}
	return ids, lastSort, pit, nil
}
