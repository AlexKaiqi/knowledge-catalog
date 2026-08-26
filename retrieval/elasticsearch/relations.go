package elasticsearch

import (
	"encoding/json"
	"fmt"
	"net/http"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func (e *esEngine) RetrieveRelations(req index.RelationRetrieveRequest) (index.RelationCandidatePage, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if req.Query.Endpoint == "" {
		return index.RelationCandidatePage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup requires an endpoint object_id")
	}
	size := req.Limit
	if size <= 0 {
		size = 500
	}
	state := pitContinuation{}
	newPIT := false
	if req.Continuation == "" {
		control, _, err := e.loadControl()
		if err != nil {
			return index.RelationCandidatePage{}, err
		}
		if control.ActiveIndex == "" || control.State != index.ProjectionStateReady {
			return index.RelationCandidatePage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "OpenSearch projection is not READY")
		}
		pit, err := e.openPIT(control.ActiveIndex)
		if err != nil {
			return index.RelationCandidatePage{}, err
		}
		state = pitContinuation{PIT: pit, Basis: kernel.CommitID(control.Basis)}
		newPIT = true
	} else {
		decoded, err := decodePITContinuation(req.Continuation)
		if err != nil {
			return index.RelationCandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid OpenSearch relation continuation")
		}
		state = decoded
	}
	ids, sortValues, err := e.searchRelationsPIT(state, req, size)
	if err != nil {
		if newPIT {
			e.closePIT(state.PIT)
		}
		return index.RelationCandidatePage{}, err
	}
	page := index.RelationCandidatePage{Exhausted: len(ids) < size}
	for i, id := range ids {
		page.Candidates = append(page.Candidates, index.CandidateRef{
			ObjectID: id, Basis: state.Basis,
			Evidence: []retrieval.LaneEvidence{{Provider: e.ProviderID(), Lane: "relation", Guarantee: string(index.GuaranteeExact), LocalRank: state.Rank + i + 1}},
		})
	}
	if page.Exhausted || len(sortValues) == 0 {
		e.closePIT(state.PIT)
		return page, nil
	}
	state.Sort = sortValues
	state.Rank += len(ids)
	page.Continuation = encodePITContinuation(state)
	return page, nil
}

func (e *esEngine) searchRelationsPIT(state pitContinuation, req index.RelationRetrieveRequest, size int) ([]knowledge.ObjectID, []any, error) {
	endpointMust := []map[string]any{{"term": map[string]any{"relation_endpoints.object_ref": string(req.Query.Endpoint)}}}
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
		return nil, nil, err
	}
	if status >= 400 {
		return nil, nil, fmt.Errorf("opensearch relation search: %s", body)
	}
	var response struct {
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
		return nil, nil, err
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
	return ids, lastSort, nil
}
