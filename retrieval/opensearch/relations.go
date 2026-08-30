package opensearch

import (
	"encoding/json"
	"fmt"
	"net/http"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func (e *openSearchEngine) RetrieveRelations(req retrieval.RelationRetrieveRequest) (retrieval.RelationCandidatePage, error) {
	e.mu.RLock()
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
	state := pitContinuation{}
	if req.Continuation == "" {
		control, _, err := e.loadControl()
		if err != nil {
			return retrieval.RelationCandidatePage{}, err
		}
		if control.ActiveIndex == "" || control.State != index.ProjectionStateReady {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "OpenSearch projection is not READY")
		}
		if kernel.CommitID(control.Basis) != req.Basis {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch relation projection basis %s does not match %s", control.Basis, req.Basis)
		}
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
		if state.Basis != req.Basis || state.Repository != req.Repository || state.Query != string(retrieval.RelationQueryDigest(req.Query)) || state.Generation == "" {
			return retrieval.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch relation continuation does not match repository, basis, query, or generation")
		}
	}
	pit, err := e.openPIT(e.prefix + "-g-" + state.Generation)
	if err != nil {
		return retrieval.RelationCandidatePage{}, err
	}
	state.PIT = pit
	ids, sortValues, nextPIT, err := e.searchRelationsPIT(state, req, size)
	if err != nil {
		e.closePIT(state.PIT)
		return retrieval.RelationCandidatePage{}, err
	}
	if nextPIT != "" {
		state.PIT = nextPIT
	}
	e.closePIT(state.PIT)
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
	status, body, err := e.do(http.MethodPost, "/_search", payload)
	if err != nil {
		return nil, nil, "", err
	}
	if status >= 400 {
		return nil, nil, "", fmt.Errorf("opensearch relation search: %s", body)
	}
	var response struct {
		PIT  string `json:"pit_id"`
		Hits struct {
			Hits []struct {
				Source struct {
					ObjectID string `json:"object_id"`
				} `json:"_source"`
				Sort []any `json:"sort"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, nil, "", err
	}
	ids := make([]knowledge.ObjectID, 0, len(response.Hits.Hits))
	var lastSort []any
	for _, hit := range response.Hits.Hits {
		if hit.Source.ObjectID == "" {
			continue
		}
		ids = append(ids, knowledge.ObjectID(hit.Source.ObjectID))
		lastSort = hit.Sort
	}
	return ids, lastSort, response.PIT, nil
}
