package opensearch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

type pitContinuation struct {
	PIT        string              `json:"pit,omitempty"`
	Basis      kernel.CommitID     `json:"basis"`
	Repository kernel.RepositoryID `json:"repository,omitempty"`
	Query      string              `json:"query,omitempty"`
	Generation string              `json:"generation,omitempty"`
	Sort       []any               `json:"sort,omitempty"`
	Rank       int                 `json:"rank,omitempty"`
	Check      kernel.Digest       `json:"check"`
}

func (e *openSearchEngine) Probe(clause retrieval.SearchClause, spec retrieval.AccessSpec) index.Capability {
	resolved, err := retrieval.ResolveSearchClause(clause, spec)
	if err != nil {
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: err.Error()}
	}
	switch resolved.Op {
	case retrieval.OpMatch, retrieval.OpEQ, retrieval.OpIN, retrieval.OpNEQ,
		retrieval.OpExists, retrieval.OpMissing, retrieval.OpPrefix, retrieval.OpContains,
		retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE,
		retrieval.OpSort:
		return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
	default:
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: "unknown operator"}
	}
}

func (e *openSearchEngine) ProbeExpression(expression retrieval.SearchExpr, spec retrieval.AccessSpec) index.Capability {
	request := retrieval.SearchWhere(expression)
	if err := retrieval.CheckSearch(request, spec); err != nil {
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: err.Error()}
	}
	for _, clause := range retrieval.SearchClauses(request) {
		capability := e.Probe(clause, spec)
		if capability.Guarantee == index.GuaranteeUnsupported {
			return capability
		}
	}
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}

func (e *openSearchEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	return e.RetrieveContext(context.Background(), req)
}

func (e *openSearchEngine) RetrieveContext(ctx context.Context, req index.RetrieveRequest) (index.CandidatePage, error) {
	if err := e.readLockContext(ctx); err != nil {
		return index.CandidatePage{}, err
	}
	defer e.mu.RUnlock()
	for _, clause := range retrieval.SearchClauses(req.Search) {
		capability := e.Probe(clause, req.Spec)
		if capability.Guarantee == index.GuaranteeUnsupported {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
	}
	size := req.Search.Limit
	if size <= 0 {
		size = retrieval.DefaultSearchLimit
	}
	if size > retrieval.MaxSearchLimit {
		return index.CandidatePage{}, kernel.Fail(kernel.ErrUsageInvalid, "search limit must be between 1 and %d", retrieval.MaxSearchLimit)
	}

	control, version, err := e.currentProjectionContext(ctx, req.Spec.Commit, req.Continuation != "")
	if err != nil {
		return index.CandidatePage{}, err
	}
	state := pitContinuation{}
	if req.Continuation == "" {
		state = pitContinuation{
			Basis: kernel.CommitID(control.Basis), Repository: req.Spec.Repository,
			Query: string(retrieval.SearchQueryDigest(req.Search)), Generation: control.Generation,
		}
	} else {
		decoded, err := decodePITContinuation(req.Continuation)
		if err != nil {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid OpenSearch continuation")
		}
		state = decoded
		if state.Basis != req.Spec.Commit || state.Repository != req.Spec.Repository ||
			state.Query != string(retrieval.SearchQueryDigest(req.Search)) || state.Generation != control.Generation {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch continuation does not match repository, basis, query, or generation")
		}
	}
	pit, err := e.openStablePITContext(ctx, control, version)
	if err != nil {
		return index.CandidatePage{}, err
	}
	state.PIT = pit

	ids, sortValues, nextPIT, err := e.searchPITContext(ctx, state, req.Search, req.Spec, size+1)
	if err != nil {
		e.closePITContext(ctx, state.PIT)
		return index.CandidatePage{}, err
	}
	if nextPIT != "" {
		state.PIT = nextPIT
	}
	e.closePITContext(ctx, state.PIT)
	hasMore := len(ids) > size
	if hasMore {
		ids = ids[:size]
		sortValues = sortValues[:size]
	}
	page := index.CandidatePage{Exhausted: !hasMore}
	for i, id := range ids {
		var providerOrder []any
		if hasExplicitSort(req.Search) && i < len(sortValues) && len(sortValues[i]) > 0 {
			orderValue, err := logicalSortValue(req.Search, req.Spec, sortValues[i][0])
			if err != nil {
				return index.CandidatePage{}, err
			}
			providerOrder = []any{orderValue}
		}
		page.Candidates = append(page.Candidates, index.CandidateRef{
			ObjectID: id, Basis: state.Basis,
			Evidence: []retrieval.LaneEvidence{{
				Provider: e.ProviderID(), Lane: osLane(req.Search), Guarantee: string(index.GuaranteeExact),
				LocalRank: state.Rank + i + 1, ProviderOrder: providerOrder,
			}},
		})
	}
	if page.Exhausted || len(sortValues) == 0 {
		return page, nil
	}
	state.Sort = sortValues[len(sortValues)-1]
	state.Rank += len(ids)
	state.PIT = ""
	page.Continuation = encodePITContinuation(state)
	return page, nil
}

func (e *openSearchEngine) openPIT(physicalIndex string) (string, error) {
	return e.openPITContext(context.Background(), physicalIndex)
}

func (e *openSearchEngine) currentProjectionContext(ctx context.Context, basis kernel.CommitID, continuing bool) (controlDoc, *controlVersion, error) {
	control, version, err := e.loadControlContext(ctx)
	if err != nil {
		return controlDoc{}, nil, err
	}
	if control.ActiveIndex == "" || control.Generation == "" || control.State != index.ProjectionStateReady {
		code := kernel.ErrTemporaryUnavailable
		if continuing {
			code = kernel.ErrPreconditionFailed
		}
		return controlDoc{}, nil, kernel.Fail(code, "OpenSearch projection is not READY; restart the search")
	}
	if kernel.CommitID(control.Basis) != basis {
		return controlDoc{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch projection basis %s does not match %s", control.Basis, basis)
	}
	return control, version, nil
}

func (e *openSearchEngine) openStablePITContext(ctx context.Context, control controlDoc, version *controlVersion) (string, error) {
	pit, err := e.openPITContext(ctx, control.ActiveIndex)
	if err != nil {
		return "", err
	}
	current, currentVersion, err := e.loadControlContext(ctx)
	if err == nil && (version == nil || currentVersion == nil || *currentVersion != *version || current != control) {
		err = kernel.Fail(kernel.ErrPreconditionFailed, "OpenSearch projection changed while opening PIT; restart the search")
	}
	if err != nil {
		e.closePITContext(ctx, pit)
		return "", err
	}
	// Updates publish UPDATING before changing documents. An unchanged control
	// version around PIT creation proves this snapshot belongs to its READY basis,
	// even when a different process owns the concurrent writer.
	return pit, nil
}

func (e *openSearchEngine) openPITContext(ctx context.Context, physicalIndex string) (string, error) {
	status, body, err := e.doContext(ctx, http.MethodPost, "/"+physicalIndex+"/_search/point_in_time?keep_alive=2m&allow_partial_pit_creation=false", nil)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("opensearch open PIT: %s", body)
	}
	var response struct {
		PIT    string         `json:"pit_id"`
		Shards *shardResponse `json:"_shards"`
	}
	if err := decodeJSON(body, &response); err != nil {
		return "", err
	}
	if err := response.Shards.check("open PIT"); err != nil {
		e.closePITContext(ctx, response.PIT)
		return "", err
	}
	if response.PIT == "" {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch open PIT returned no pit_id")
	}
	return response.PIT, nil
}

func (e *openSearchEngine) closePIT(pit string) {
	e.closePITContext(context.Background(), pit)
}

func (e *openSearchEngine) closePITContext(ctx context.Context, pit string) {
	if pit == "" {
		return
	}
	cleanup, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, _, _ = e.doContext(cleanup, http.MethodDelete, "/_search/point_in_time", map[string]any{"pit_id": pit})
}

func (e *openSearchEngine) searchPIT(state pitContinuation, req retrieval.SearchRequest, spec retrieval.AccessSpec, size int) ([]knowledge.ObjectID, [][]any, string, error) {
	return e.searchPITContext(context.Background(), state, req, spec, size)
}

func (e *openSearchEngine) searchPITContext(ctx context.Context, state pitContinuation, req retrieval.SearchRequest, spec retrieval.AccessSpec, size int) ([]knowledge.ObjectID, [][]any, string, error) {
	query, scoring, err := osQuery(req, spec)
	if err != nil {
		return nil, nil, "", err
	}
	sortSpec, explicitSort, err := osSort(req, spec)
	if err != nil {
		return nil, nil, "", err
	}
	if scoring && !explicitSort {
		sortSpec = append([]any{map[string]any{"_score": map[string]any{"order": "desc"}}}, sortSpec...)
	}
	payload := map[string]any{
		"size": size, "_source": []string{"object_id"}, "track_total_hits": false,
		"pit":   map[string]any{"id": state.PIT, "keep_alive": "2m"},
		"query": query, "sort": sortSpec,
	}
	if len(state.Sort) > 0 {
		payload["search_after"] = state.Sort
	}
	status, body, err := e.doContext(ctx, http.MethodPost, "/_search?allow_partial_search_results=false", payload)
	if err != nil {
		return nil, nil, "", err
	}
	if status >= 400 {
		return nil, nil, "", fmt.Errorf("opensearch search: %s", body)
	}
	return decodeSearchResponse(body, len(sortSpec))
}

// osSort freezes the logical reduction for multi-valued fields: ascending
// uses the minimum value, descending uses the maximum, and missing values are
// always last. object_id is the total-order tie breaker.
func osSort(req retrieval.SearchRequest, spec retrieval.AccessSpec) ([]any, bool, error) {
	tieBreak := map[string]any{"object_id": map[string]any{"order": "asc"}}
	clause, ok := retrieval.SearchSortClause(req)
	if !ok {
		return []any{tieBreak}, false, nil
	}
	if clause.Field == nil {
		return nil, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SORT field is unresolved")
	}
	field, err := spec.ResolveField(*clause.Field)
	if err != nil {
		return nil, false, err
	}
	slot, err := sortSlot(field.Type)
	if err != nil {
		return nil, false, err
	}
	order := strings.ToLower(strings.TrimSpace(clause.Order))
	if order == "" {
		order = "asc"
	}
	mode := "min"
	if order == "desc" {
		mode = "max"
	}
	business := map[string]any{"cells." + slot: map[string]any{
		"order": order, "mode": mode, "missing": "_last",
		"nested": map[string]any{
			"path":   "cells",
			"filter": map[string]any{"term": map[string]any{"cells.field": clause.Path}},
		},
	}}
	return []any{business, tieBreak}, true, nil
}

func sortSlot(fieldType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "", "string", "object_ref", "object_ref_list":
		return "string_value", nil
	case "bool", "boolean":
		return "boolean_value", nil
	case "int", "integer", "long":
		return "long_value", nil
	case "number", "float", "double":
		return "double_value", nil
	case "date", "datetime", "timestamp":
		return "date_value", nil
	default:
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "OpenSearch does not support sorting scalar type %q", fieldType)
	}
}

func hasExplicitSort(req retrieval.SearchRequest) bool {
	_, ok := retrieval.SearchSortClause(req)
	return ok
}

func osQuery(req retrieval.SearchRequest, spec retrieval.AccessSpec) (map[string]any, bool, error) {
	expression, ok := retrieval.SearchPredicate(req)
	if !ok {
		return nil, false, kernel.Fail(kernel.ErrUsageInvalid, "search requires a locating clause")
	}
	return osExpression(expression, spec)
}

func osExpression(expression retrieval.SearchExpr, spec retrieval.AccessSpec) (map[string]any, bool, error) {
	if expression.Clause != nil {
		clause := *expression.Clause
		fieldType := ""
		if clause.Field != nil {
			field, err := spec.ResolveField(*clause.Field)
			if err != nil {
				return nil, false, err
			}
			fieldType = field.Type
		}
		return osClause(clause, fieldType)
	}
	children := expression.All
	isAny := false
	if expression.Any != nil {
		children = expression.Any
		isAny = true
	}
	must := []map[string]any{}
	filters := []map[string]any{}
	should := []map[string]any{}
	hasScoring := false
	for _, child := range children {
		query, scoring, err := osExpression(child, spec)
		if err != nil {
			return nil, false, err
		}
		hasScoring = hasScoring || scoring
		if isAny {
			if !scoring {
				query = map[string]any{"bool": map[string]any{"filter": []map[string]any{query}}}
			}
			should = append(should, query)
			continue
		}
		if scoring {
			must = append(must, query)
		} else {
			filters = append(filters, query)
		}
	}
	if isAny {
		return map[string]any{"bool": map[string]any{
			"should": should, "minimum_should_match": 1,
		}}, hasScoring, nil
	}
	return map[string]any{"bool": map[string]any{"must": must, "filter": filters}}, hasScoring, nil
}

func osClause(clause retrieval.SearchClause, fieldType string) (map[string]any, bool, error) {
	nested := func(conditions ...map[string]any) map[string]any {
		return map[string]any{"nested": map[string]any{
			"path": "cells", "query": map[string]any{"bool": map[string]any{"must": conditions}},
		}}
	}
	fieldCondition := map[string]any{"term": map[string]any{"cells.field": clause.Path}}
	matchQuery := func(field string) map[string]any {
		if clause.Mode == retrieval.MatchPhrase {
			return map[string]any{"match_phrase": map[string]any{field: clause.Value}}
		}
		operator := "and"
		if clause.Mode == retrieval.MatchAnyTerms {
			operator = "or"
		}
		return map[string]any{"match": map[string]any{field: map[string]any{"query": clause.Value, "operator": operator}}}
	}
	slot, value, err := typedQueryValue(fieldType, clause.Value)
	switch clause.Op {
	case retrieval.OpMatch:
		if clause.Path == "" {
			return matchQuery("all_text"), true, nil
		}
		return nested(fieldCondition, matchQuery("cells.text_value")), true, nil
	case retrieval.OpExists:
		return nested(fieldCondition), false, nil
	case retrieval.OpMissing:
		return map[string]any{"bool": map[string]any{
			"filter":   []map[string]any{{"term": map[string]any{"eligible_fields": clause.Path}}},
			"must_not": []map[string]any{nested(fieldCondition)},
		}}, false, nil
	case retrieval.OpEQ:
		if err != nil {
			return nil, false, err
		}
		return nested(fieldCondition, map[string]any{"term": map[string]any{"cells." + slot: value}}), false, nil
	case retrieval.OpIN:
		values := make([]any, 0, len(clause.Values))
		for _, raw := range clause.Values {
			_, item, valueErr := typedQueryValue(fieldType, raw)
			if valueErr != nil {
				return nil, false, valueErr
			}
			values = append(values, item)
		}
		return nested(fieldCondition, map[string]any{"terms": map[string]any{"cells." + slot: values}}), false, nil
	case retrieval.OpNEQ:
		if err != nil {
			return nil, false, err
		}
		equal := nested(fieldCondition, map[string]any{"term": map[string]any{"cells." + slot: value}})
		return map[string]any{"bool": map[string]any{
			"filter": []map[string]any{nested(fieldCondition)}, "must_not": []map[string]any{equal},
		}}, false, nil
	case retrieval.OpPrefix:
		return nested(fieldCondition, map[string]any{"prefix": map[string]any{"cells.string_value": clause.Value}}), false, nil
	case retrieval.OpContains:
		return nested(fieldCondition, map[string]any{"wildcard": map[string]any{
			"cells.string_value": map[string]any{"value": retrieval.WildcardContainsPattern(clause.Value)},
		}}), false, nil
	case retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE:
		if err != nil {
			return nil, false, err
		}
		rangeOp := strings.ToLower(string(clause.Op))
		return nested(fieldCondition, map[string]any{"range": map[string]any{"cells." + slot: map[string]any{rangeOp: value}}}), false, nil
	default:
		return nil, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "OpenSearch projection does not implement %s", clause.Op)
	}
}

func typedQueryValue(fieldType, normalized string) (string, any, error) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "", "string", "object_ref", "object_ref_list":
		return "string_value", normalized, nil
	case "bool", "boolean":
		value, err := strconv.ParseBool(normalized)
		return "boolean_value", value, err
	case "int", "integer", "long":
		value, err := strconv.ParseInt(normalized, 10, 64)
		return "long_value", value, err
	case "number", "float", "double":
		value, err := strconv.ParseFloat(normalized, 64)
		return "double_value", value, err
	case "date", "datetime", "timestamp":
		value, err := encodeTemporalKey(normalized)
		return "date_value", value, err
	default:
		return "", nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "OpenSearch does not support scalar type %q", fieldType)
	}
}

func osLane(req retrieval.SearchRequest) string {
	if retrieval.SearchHasOp(req, retrieval.OpMatch) {
		return "text"
	}
	return "filter"
}

func encodePITContinuation(state pitContinuation) string {
	state.Check = ""
	state.Check = continuationDigest(state)
	body, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodePITContinuation(encoded string) (pitContinuation, error) {
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return pitContinuation{}, err
	}
	var state pitContinuation
	if err := decodeJSON(body, &state); err != nil {
		return pitContinuation{}, err
	}
	want := state.Check
	state.Check = ""
	if want == "" || continuationDigest(state) != want {
		return pitContinuation{}, fmt.Errorf("continuation checksum mismatch")
	}
	state.Check = want
	if state.Basis == "" || state.Generation == "" {
		return pitContinuation{}, fmt.Errorf("missing PIT continuation fields")
	}
	return state, nil
}

func continuationDigest(state pitContinuation) kernel.Digest {
	// Hash the exact serialized value, including integer boundaries. Generic
	// canonicalization of an interface tree must not round its JSON numbers.
	body, _ := json.Marshal(state)
	return kernel.CanonicalDigest(string(body))
}
