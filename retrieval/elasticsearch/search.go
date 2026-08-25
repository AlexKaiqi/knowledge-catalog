package elasticsearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func (e *esEngine) Probe(clause reader.SearchClause, spec reader.AccessSpec) index.Capability {
	resolved, err := reader.ResolveSearchClause(clause, spec)
	if err != nil {
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: err.Error()}
	}
	switch resolved.Op {
	case reader.OpMatch, reader.OpEQ, reader.OpIN, reader.OpExists, reader.OpMissing, reader.OpPrefix:
		return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
	case reader.OpSort, reader.OpNEQ, reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: "operator is not implemented by elasticsearch provider"}
	default:
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: "unknown operator"}
	}
}

func (e *esEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	guarantee := index.GuaranteeExact
	for _, clause := range req.Search.Clauses {
		capability := e.Probe(clause, req.Spec)
		if capability.Guarantee == index.GuaranteeUnsupported {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
		if capability.Guarantee == index.GuaranteeSuperset {
			guarantee = capability.Guarantee
		}
	}
	from := 0
	if req.Continuation != "" {
		var err error
		from, err = strconv.Atoi(req.Continuation)
		if err != nil || from < 0 {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid elasticsearch continuation")
		}
	}
	size := req.Search.Limit
	if size <= 0 {
		size = 500
	}
	ids, err := e.searchIDs(req.Search, from, size)
	if err != nil {
		return index.CandidatePage{}, err
	}
	meta, err := e.LoadMeta()
	if err != nil {
		return index.CandidatePage{}, err
	}
	page := index.CandidatePage{Exhausted: len(ids) < size}
	for i, id := range ids {
		page.Candidates = append(page.Candidates, index.CandidateRef{
			ObjectID: id, Basis: meta.Basis,
			Evidence: []reader.LaneEvidence{{Provider: e.ProviderID(), Lane: esLane(req.Search), Guarantee: string(guarantee), LocalRank: from + i + 1}},
		})
	}
	if !page.Exhausted {
		page.Continuation = strconv.Itoa(from + len(ids))
	}
	return page, nil
}

func (e *esEngine) searchIDs(req reader.SearchRequest, from, size int) ([]knowledge.ObjectID, error) {
	var filters []map[string]any
	for _, clause := range req.Clauses {
		if clause.Op == reader.OpSort {
			continue
		}
		query, err := esClause(clause)
		if err != nil {
			return nil, err
		}
		filters = append(filters, query)
	}
	if len(filters) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search requires a locating clause")
	}
	payload := map[string]any{
		"from": from, "size": size, "_source": []string{"object_id"},
		"query": map[string]any{"bool": map[string]any{"filter": filters}},
	}
	status, body, err := e.do(http.MethodPost, "/"+e.index+"/_search", payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("elasticsearch search: %s", body)
	}
	var out struct {
		Hits struct {
			Hits []struct {
				Source struct {
					ObjectID string `json:"object_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	ids := make([]knowledge.ObjectID, 0, len(out.Hits.Hits))
	for _, hit := range out.Hits.Hits {
		if hit.Source.ObjectID != "" {
			ids = append(ids, knowledge.ObjectID(hit.Source.ObjectID))
		}
	}
	return ids, nil
}

func esLane(req reader.SearchRequest) string {
	for _, clause := range req.Clauses {
		if clause.Op == reader.OpMatch {
			return "text"
		}
	}
	return "filter"
}

func esClause(clause reader.SearchClause) (map[string]any, error) {
	nested := func(path, value string) map[string]any {
		return map[string]any{"nested": map[string]any{
			"path": "fields",
			"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
				{"term": map[string]any{"fields.path": path}},
				{"term": map[string]any{"fields.value": value}},
			}}},
		}}
	}
	switch clause.Op {
	case reader.OpMatch:
		matchQuery := func(field string) map[string]any {
			if clause.Mode == reader.MatchPhrase {
				return map[string]any{"match_phrase": map[string]any{field: clause.Value}}
			}
			operator := "and"
			if clause.Mode == reader.MatchAnyTerms {
				operator = "or"
			}
			return map[string]any{"match": map[string]any{field: map[string]any{"query": clause.Value, "operator": operator}}}
		}
		if clause.Path == "" {
			return matchQuery("value_text"), nil
		}
		return map[string]any{"nested": map[string]any{
			"path": "fields",
			"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
				{"term": map[string]any{"fields.path": clause.Path}},
				matchQuery("fields.text_value"),
			}}},
		}}, nil
	case reader.OpEQ:
		return nested(clause.Path, clause.Value), nil
	case reader.OpIN:
		should := make([]map[string]any, 0, len(clause.Values))
		for _, value := range clause.Values {
			should = append(should, nested(clause.Path, value))
		}
		return map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}}, nil
	case reader.OpExists:
		return map[string]any{"nested": map[string]any{
			"path": "fields", "query": map[string]any{"term": map[string]any{"fields.path": clause.Path}},
		}}, nil
	case reader.OpMissing:
		return map[string]any{"bool": map[string]any{"must_not": []map[string]any{{
			"nested": map[string]any{"path": "fields", "query": map[string]any{"term": map[string]any{"fields.path": clause.Path}}},
		}}}}, nil
	case reader.OpPrefix:
		return map[string]any{"nested": map[string]any{
			"path": "fields",
			"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
				{"term": map[string]any{"fields.path": clause.Path}},
				{"prefix": map[string]any{"fields.value": clause.Value}},
			}}},
		}}, nil
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "elasticsearch projection does not implement %s", clause.Op)
	}
}
