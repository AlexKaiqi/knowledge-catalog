package opensearch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

func responseTestSearch() (retrieval.SearchRequest, retrieval.AccessSpec) {
	return retrieval.SearchOf(retrieval.SearchMATCH("a")), retrieval.AccessSpec{Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "text"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}
}

func TestOpenSearchIncompleteResponsesCannotProduceCandidates(t *testing.T) {
	for name, metadata := range map[string]string{
		"timeout":           `"timed_out":true,"_shards":{"total":2,"successful":2,"failed":0}`,
		"failed shard":      `"timed_out":false,"_shards":{"total":2,"successful":1,"failed":1}`,
		"unaccounted shard": `"timed_out":false,"_shards":{"total":2,"successful":1,"failed":0}`,
		"early termination": `"timed_out":false,"terminated_early":true,"_shards":{"total":2,"successful":2,"failed":0}`,
		"missing metadata":  `"took":1`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{%s,"hits":{"hits":[{"_source":{"object_id":"entity/a"},"sort":["entity/a"]}]}}`, metadata)
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			query, spec := responseTestSearch()
			ids, _, _, err := engine.searchPIT(pitContinuation{PIT: "pit"}, query, spec, 2)
			if len(ids) != 0 || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
				t.Errorf("SEARCH must reject incomplete result: ids=%v err=%v", ids, err)
			}
			ids, _, _, err = engine.searchRelationsPIT(pitContinuation{PIT: "pit"}, retrieval.RelationRetrieveRequest{}, 2)
			if len(ids) != 0 || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
				t.Errorf("RELATIONS must reject incomplete result: ids=%v err=%v", ids, err)
			}
		})
	}
}

func TestOpenSearchPartialPITIsRejectedAndClosed(t *testing.T) {
	closed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			closed = true
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"pit_id":"partial","_shards":{"total":2,"successful":1,"failed":1}}`)
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	pit, err := engine.openPIT("generation")
	if pit != "" || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !closed {
		t.Fatalf("partial PIT must fail and release resources: pit=%q err=%v closed=%v", pit, err, closed)
	}
}

func TestOpenSearchIntegerSortSurvivesSearchAndContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[{"_source":{"object_id":"entity/a"},"sort":[9007199254740993,"entity/a"]}]}}`)
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	query, spec := responseTestSearch()
	_, sorts, _, err := engine.searchPIT(pitContinuation{PIT: "pit"}, query, spec, 2)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(sorts[0])
	if string(encoded) != `[9007199254740993,"entity/a"]` {
		t.Errorf("search rounded sort tuple: %s", encoded)
	}
	state := pitContinuation{Basis: "c1", Generation: "g1", Sort: []any{json.Number("9007199254740993"), "entity/a"}}
	token := encodePITContinuation(state)
	decoded, err := decodePITContinuation(token)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(decoded.Sort)
	if string(encoded) != `[9007199254740993,"entity/a"]` {
		t.Errorf("continuation rounded sort tuple: %s", encoded)
	}
	body, _ := base64.RawURLEncoding.DecodeString(token)
	body = []byte(strings.Replace(string(body), "9007199254740993", "9007199254740992", 1))
	if _, err := decodePITContinuation(base64.RawURLEncoding.EncodeToString(body)); err == nil {
		t.Error("checksum accepted a different integer boundary")
	}
}

func TestOpenSearchTemporalKeysPreserveChronology(t *testing.T) {
	values := []string{"0000-01-01T00:00:00Z", "1670-01-01T00:00:00Z", "1970-01-01T00:00:00Z", "2026-01-01T00:00:00.000000001Z", "2026-01-01T00:00:00.000000002Z", "2026-01-01T00:00:00.1Z", "2026-01-01T00:00:00.11Z", "9999-12-31T23:59:59.999999999Z"}
	previous := ""
	for _, value := range values {
		_, key, err := typedQueryValue("timestamp", value)
		if err != nil {
			t.Fatal(err)
		}
		if previous != "" && previous >= key.(string) {
			t.Errorf("temporal key order reversed at %s: %q >= %q", value, previous, key)
		}
		previous = key.(string)
	}
	_, utc, err := typedQueryValue("timestamp", "2026-01-01T00:00:00.000000001Z")
	if err != nil {
		t.Fatal(err)
	}
	_, offset, err := typedQueryValue("timestamp", "2026-01-01T08:00:00.000000001+08:00")
	if err != nil || utc != offset {
		t.Errorf("same instant must have same key: %v != %v, err=%v", utc, offset, err)
	}
}

func TestOpenSearchApplyWaitsForEveryTouchedShard(t *testing.T) {
	control := controlDoc{Repository: "repo", ActiveIndex: "generation", Generation: "g1", State: index.ProjectionStateReady, Basis: "c1"}
	dirty := map[int]bool{}
	batch := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"_source": control, "_seq_no": 1, "_primary_term": 1})
		case r.Method == http.MethodPut:
			var next controlDoc
			_ = json.NewDecoder(r.Body).Decode(&next)
			if next.State == index.ProjectionStateReady && len(dirty) > 0 {
				t.Errorf("READY published while earlier batch shards remain invisible: %v", dirty)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"_seq_no": 2, "_primary_term": 1})
		case r.URL.Path == "/_bulk":
			// Each batch touches a distinct shard. OpenSearch wait_for only waits
			// for shards touched by this request, not earlier bulk requests.
			batch++
			dirty[batch] = true
			if r.URL.Query().Get("refresh") == "wait_for" {
				delete(dirty, batch)
			}
			body, _ := io.ReadAll(r.Body)
			count := strings.Count(string(body), "\n") / 2
			items := make([]any, count)
			for i := range items {
				items[i] = map[string]any{"index": map[string]any{"status": 201, "result": "created"}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": false, "items": items})
		default:
			http.Error(w, "unexpected operation", 500)
		}
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "control", repository: "repo"}
	docs := make([]index.CompiledDoc, 501)
	for i := range docs {
		docs[i].ObjectID = knowledge.ObjectID(fmt.Sprintf("entity/%d", i))
	}
	if err := engine.Apply(docs, nil, index.Meta{Basis: "c2"}); err != nil {
		t.Fatal(err)
	}
	if batch != 2 {
		t.Fatalf("expected multiple shards across two batches, got %d", batch)
	}
}

func TestOpenSearchReadinessRejectsPartialShardAcknowledgements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"count":0,"_shards":{"total":2,"successful":1,"failed":1}}`)
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	if err := engine.refresh("generation"); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Errorf("refresh accepted partial acknowledgement: %v", err)
	}
	if _, err := engine.countIndex("generation"); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Errorf("count accepted partial acknowledgement: %v", err)
	}
}

func TestOpenSearchFirstPageRequiresRequestedBasis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"_source": controlDoc{State: index.ProjectionStateReady, ActiveIndex: "generation", Generation: "g1", Basis: "c1"}})
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	query, spec := responseTestSearch()
	spec.Commit = "c2"
	if _, err := engine.Retrieve(index.RetrieveRequest{Search: query, Spec: spec}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("basis mismatch: %v", err)
	}
}

func TestOpenSearchRetrievalCancellationIncludesProjectionLock(t *testing.T) {
	engine := &openSearchEngine{http: http.DefaultClient, base: "http://127.0.0.1"}
	engine.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := engine.RetrieveContext(ctx, index.RetrieveRequest{}); result <- err }()
	select {
	case err := <-result:
		engine.mu.Unlock()
		if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
			t.Fatalf("cancelled retrieval: %v", err)
		}
	case <-time.After(150 * time.Millisecond):
		engine.mu.Unlock()
		<-result
		t.Fatal("retrieval deadline cannot interrupt projection lock wait")
	}
}

func TestOpenSearchTemporalProjectionQueryAndLogicalSortAgree(t *testing.T) {
	for _, value := range []string{"0000-01-01", "9999-12-31", "0000-01-01T00:00:00Z", "9999-12-31T23:59:59.999999999Z", "2026-01-01T00:00:00.000000001Z", "2026-01-01T00:00:00.000000002Z", "2026-01-01T00:00:00.1Z", "2026-01-01T00:00:00.11Z", "-0001-12-31T23:00:00Z", "10000-01-01T00:59:59Z"} {
		t.Run(value, func(t *testing.T) {
			fieldType := "timestamp"
			if len(value) == 10 {
				fieldType = "date"
			}
			field := retrieval.AccessField{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "time"}, Type: fieldType, Access: []reader.AccessHint{reader.HintFilter, reader.HintSort}}
			spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{field}}
			doc, err := encodeDoc(index.CompiledDoc{Cells: []index.ProjectionCell{{Field: field.Key(), DateValue: value}}})
			if err != nil {
				t.Fatal(err)
			}
			_, key, err := typedQueryValue(fieldType, value)
			if err != nil || doc.Cells[0].DateValue != key {
				t.Fatalf("projection/query differ: %#v key=%v err=%v", doc, key, err)
			}
			for _, op := range []retrieval.SearchOp{retrieval.OpEQ, retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE} {
				query, _, err := osClause(retrieval.SearchClause{Op: op, Path: field.Key(), Value: value}, fieldType)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(query)
				if !strings.Contains(string(body), key.(string)) {
					t.Fatalf("%s does not compare the projected temporal key: %s", op, body)
				}
			}
			sortClause, err := retrieval.ResolveSearchClause(retrieval.SearchSORT("time", "asc"), spec)
			if err != nil {
				t.Fatal(err)
			}
			logical, err := logicalSortValue(retrieval.SearchOf(sortClause), spec, key)
			if err != nil || logical != value {
				t.Fatalf("provider order leaked physical representation: logical=%v err=%v", logical, err)
			}
		})
	}
	engine := &openSearchEngine{}
	cellProperties := engine.projectionMapping()["mappings"].(map[string]any)["properties"].(map[string]any)["cells"].(map[string]any)["properties"].(map[string]any)
	if cellProperties["date_value"].(map[string]any)["type"] != "keyword" {
		t.Fatal("physical temporal mapping must preserve exact sortable keys")
	}
}

func TestOpenSearchCancellationReachesEveryReadStage(t *testing.T) {
	for _, stage := range []string{"control", "pit", "search"} {
		t.Run(stage, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := "control"
				if strings.Contains(r.URL.Path, "point_in_time") {
					current = "pit"
				}
				if r.URL.Path == "/_search" {
					current = "search"
				}
				if r.Method == http.MethodDelete {
					fmt.Fprint(w, `{}`)
					return
				}
				if current == stage {
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				switch current {
				case "control":
					fmt.Fprint(w, `{"_source":{"active_index":"generation","generation":"g1","state":"READY","basis":"c1"}}`)
				case "pit":
					fmt.Fprint(w, `{"pit_id":"pit","_shards":{"total":1,"successful":1,"failed":0}}`)
				}
			}))
			defer server.Close()
			defer close(release)
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			query, spec := responseTestSearch()
			spec.Commit = "c1"
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := engine.RetrieveContext(ctx, index.RetrieveRequest{Search: query, Spec: spec})
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || time.Since(started) > 500*time.Millisecond {
				t.Fatalf("cancellation did not bound %s: %v (%v)", stage, err, time.Since(started))
			}
		})
	}
}

func TestOpenSearchContinuationRequiresCurrentPublication(t *testing.T) {
	for _, changed := range []controlDoc{
		{ActiveIndex: "generation", Generation: "g1", Basis: "c2", State: index.ProjectionStateReady},
		{ActiveIndex: "generation", Generation: "g1", Basis: "c1", State: index.ProjectionStateUpdating},
		{ActiveIndex: "generation2", Generation: "g2", Basis: "c1", State: index.ProjectionStateReady},
	} {
		t.Run(changed.Generation+changed.Basis+changed.State, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(map[string]any{"_source": changed, "_seq_no": 2, "_primary_term": 1})
				case r.URL.Path == "/_search":
					fmt.Fprint(w, `{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[]}}`)
				default:
					fmt.Fprint(w, `{"pit_id":"pit","_shards":{"total":1,"successful":1,"failed":0}}`)
				}
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			query, spec := responseTestSearch()
			spec.Commit = "c1"
			spec.Repository = "repo"
			token := encodePITContinuation(pitContinuation{Basis: "c1", Repository: "repo", Generation: "g1", Query: string(retrieval.SearchQueryDigest(query))})
			if _, err := engine.Retrieve(index.RetrieveRequest{Search: query, Spec: spec, Continuation: token}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("continuation followed changed publication: %v", err)
			}
			req := retrieval.RelationRetrieveRequest{Repository: "repo", Basis: "c1", Query: retrieval.RelationQuery{Endpoint: knowledge.KnowledgeRef{Repository: "repo", Object: "entity/a"}}}
			req.Continuation = encodePITContinuation(pitContinuation{Basis: "c1", Repository: "repo", Generation: "g1", Query: string(retrieval.RelationQueryDigest(req.Query))})
			if _, err := engine.RetrieveRelations(req); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("relation continuation followed changed publication: %v", err)
			}
		})
	}
}

func TestOpenSearchPITCreationRejectsConcurrentPublication(t *testing.T) {
	for _, relation := range []bool{false, true} {
		t.Run(fmt.Sprint(relation), func(t *testing.T) {
			reads, searches, closed := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet:
					reads++
					// A different engine starts and finishes Apply while PIT is
					// opening. Basis alone is insufficient if a repair uses the same basis.
					seq := 1
					if reads > 1 {
						seq = 3
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"_source": controlDoc{ActiveIndex: "generation", Generation: "g1", Basis: "c1", State: index.ProjectionStateReady}, "_seq_no": seq, "_primary_term": 1})
				case r.Method == http.MethodDelete:
					closed++
					fmt.Fprint(w, `{}`)
				case r.URL.Path == "/_search":
					searches++
					fmt.Fprint(w, `{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[]}}`)
				default:
					fmt.Fprint(w, `{"pit_id":"pit","_shards":{"total":1,"successful":1,"failed":0}}`)
				}
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			var err error
			if relation {
				_, err = engine.RetrieveRelations(retrieval.RelationRetrieveRequest{Repository: "repo", Basis: "c1", Query: retrieval.RelationQuery{Endpoint: knowledge.KnowledgeRef{Repository: "repo", Object: "entity/a"}}})
			} else {
				query, spec := responseTestSearch()
				spec.Commit = "c1"
				_, err = engine.Retrieve(index.RetrieveRequest{Search: query, Spec: spec})
			}
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || searches != 0 || closed != 1 {
				t.Fatalf("concurrent publication crossed PIT creation: err=%v searches=%d closed=%d", err, searches, closed)
			}
		})
	}
}

func TestOpenSearchObjectReferencesUseExactKeywordCells(t *testing.T) {
	for _, fieldType := range []string{"object_ref", "object_ref_list"} {
		t.Run(fieldType, func(t *testing.T) {
			ref := "concept/refund"
			field := retrieval.AccessField{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "concept"}, Type: fieldType, Access: []reader.AccessHint{reader.HintFilter, reader.HintSort}}
			spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{field}}
			query, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchEQ("concept", ref), retrieval.SearchSORT("concept", "asc")), spec)
			if err != nil {
				t.Fatal(err)
			}
			translated, _, err := osQuery(query, spec)
			if err != nil {
				t.Errorf("typed reference EQ: %v", err)
			} else if !strings.Contains(fmt.Sprint(translated), "cells.string_value:"+ref) {
				t.Errorf("reference query must use exact projected string value: %v", translated)
			}
			if _, _, err := osSort(query, spec); err != nil {
				t.Errorf("typed reference sort: %v", err)
			}
		})
	}
}

func TestOpenSearchOpenDefersNetworkAndControlMaintenance(t *testing.T) {
	calls, controlCreated := 0, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/"+controlIndexName:
			controlCreated = true
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodGet:
			http.NotFound(w, r)
		case r.Method == http.MethodPut:
			if !controlCreated {
				t.Error("first rebuild writes without preparing the control index")
			}
			fmt.Fprint(w, `{"_seq_no":1,"_primary_term":1}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	engine, err := Open(Config{URL: server.URL})("", "repo")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if calls != 0 {
		t.Errorf("opening a read handle performed %d network/maintenance requests", calls)
	}
	session, err := engine.(index.StreamingProjectionMaintainer).BeginRebuild(index.Meta{Basis: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if !controlCreated {
		t.Error("first rebuild did not prepare control index")
	}
	_ = session.Abort(fmt.Errorf("test abort"))
}
