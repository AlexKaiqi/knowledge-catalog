package opensearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/knowledge/reader"
	"kc/retrieval"
)

func TestOpenSearchTransientHTTPStatusIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "try later", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	status, _, err := engine.doBytes(http.MethodGet, "/_cluster/health", nil, "")
	if status != http.StatusServiceUnavailable || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("status=%d err=%v code=%s", status, err, kernel.CodeOf(err))
	}
}

func TestOpenSearchProbeTypedSubset(t *testing.T) {
	engine := &openSearchEngine{}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "name"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "n"}, Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "when"}, Type: "string", Access: []reader.AccessHint{reader.HintSort}},
	}}
	for _, clause := range []retrieval.SearchClause{
		retrieval.SearchMATCHMode("daily events", retrieval.MatchAllTerms),
		retrieval.SearchMATCHMode("daily events", retrieval.MatchAnyTerms),
		retrieval.SearchMATCHMode("daily events", retrieval.MatchPhrase),
		retrieval.SearchEQ("name", "customer.orders"),
		retrieval.SearchIN("name", "a", "b"),
		retrieval.SearchEXISTS("name"),
		retrieval.SearchMISSING("name"),
		retrieval.SearchPREFIX("name", "customer."),
		retrieval.SearchCONTAINS("name", "tomer"),
		retrieval.SearchNEQ("name", "x"),
		retrieval.SearchRange(retrieval.OpGT, "n", "1"),
	} {
		if capability := engine.Probe(clause, spec); capability.Guarantee != index.GuaranteeExact {
			t.Fatalf("%s: %#v", clause.Op, capability)
		}
	}
	if capability := engine.Probe(retrieval.SearchSORT("when", "asc"), spec); capability.Guarantee != index.GuaranteeExact {
		t.Fatalf("SORT has a frozen multi-value reduction policy: %#v", capability)
	}
}

func TestOpenSearchSortFreezesReductionAndMissingPolicy(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "when"}, Type: "string",
		Access: []reader.AccessHint{reader.HintSort},
	}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{field}}
	clause, err := retrieval.ResolveSearchClause(retrieval.SearchSORT("when", "desc"), spec)
	if err != nil {
		t.Fatal(err)
	}
	sorts, explicit, err := osSort(retrieval.SearchOf(clause), spec)
	if err != nil || !explicit || len(sorts) != 2 {
		t.Fatalf("sorts=%#v explicit=%v err=%v", sorts, explicit, err)
	}
	encoded := fmt.Sprint(sorts[0])
	for _, invariant := range []string{"order:desc", "mode:max", "missing:_last", "cells.field:" + clause.Path} {
		if !strings.Contains(encoded, invariant) {
			t.Fatalf("sort must contain %q: %s", invariant, encoded)
		}
	}
}

func TestOpenSearchContainsUsesEscapedKeywordWildcard(t *testing.T) {
	query, scoring, err := osClause(retrieval.SearchCONTAINS("name", `a*b?c\d`), "string")
	if err != nil || scoring {
		t.Fatalf("query=%#v scoring=%v err=%v", query, scoring, err)
	}
	body, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	encodedPattern, err := json.Marshal(retrieval.WildcardContainsPattern(`a*b?c\d`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `"wildcard"`) || !strings.Contains(text, string(encodedPattern)) {
		t.Fatalf("CONTAINS must be an escaped keyword wildcard: %s", text)
	}
	if strings.Contains(text, `"prefix"`) {
		t.Fatalf("CONTAINS must not reuse PREFIX: %s", text)
	}
}

func TestOpenSearchMissingRequiresApplicability(t *testing.T) {
	clause := retrieval.SearchClause{Op: retrieval.OpMissing, Path: "schema/t\x1f\x1fname"}
	query, _, err := osClause(clause, "string")
	if err != nil {
		t.Fatal(err)
	}
	encoded := fmt.Sprint(query)
	if !strings.Contains(encoded, "eligible_fields") || !strings.Contains(encoded, "must_not") {
		t.Fatalf("MISSING must distinguish absent from inapplicable: %#v", query)
	}
}

func TestOpenSearchTranslatesNestedAllAnyExpression(t *testing.T) {
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "db"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "owner"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
	}}
	req := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchMATCH("runbook")),
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
		),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
		retrieval.SearchLeaf(retrieval.SearchCONTAINS("db", "prod*")),
	))
	resolved, err := retrieval.ResolveSearch(req, spec)
	if err != nil {
		t.Fatal(err)
	}
	query, scoring, err := osQuery(resolved, spec)
	if err != nil || !scoring {
		t.Fatalf("query=%#v scoring=%v err=%v", query, scoring, err)
	}
	encoded, _ := json.Marshal(query)
	text := string(encoded)
	for _, required := range []string{"minimum_should_match", "should", "filter", "all_text", "cells.string_value", `"wildcard"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("query must contain %q: %s", required, text)
		}
	}
	encodedPattern, err := json.Marshal(retrieval.WildcardContainsPattern("prod*"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, string(encodedPattern)) {
		t.Fatalf("expression CONTAINS must keep literal star: %s", text)
	}
}

func TestOpenSearchDocumentIDIsCollisionSafe(t *testing.T) {
	if documentID("a/b:c") == documentID("a:b/c") {
		t.Fatal("lossy path sanitization must not identify projection documents")
	}
}

func TestOpenSearchProjectionScaleSettingsAffectPhysicalDigest(t *testing.T) {
	cfg := (Config{}).WithDefaults()
	if cfg.PrimaryShards != 8 || cfg.RefreshInterval != "1s" {
		t.Fatalf("scale-first defaults: %#v", cfg)
	}
	left := &openSearchEngine{primaryShards: 8, replicas: 1, refreshInterval: "1s"}
	right := &openSearchEngine{primaryShards: 16, replicas: 1, refreshInterval: "1s"}
	if left.PhysicalDigest() == right.PhysicalDigest() {
		t.Fatal("shard topology must be part of physical projection identity")
	}
	settings := left.projectionMapping()["settings"].(map[string]any)["index"].(map[string]any)
	if settings["number_of_shards"] != 8 || settings["number_of_replicas"] != 1 || settings["refresh_interval"] != "1s" {
		t.Fatalf("projection settings: %#v", settings)
	}
}

func TestOpenSearchWarmRebuildKeepsReadyGenerationQueryable(t *testing.T) {
	old := controlDoc{
		Repository: "kr://acme/public/core", ActiveIndex: "kc-proj-test-g-old", Generation: "old",
		State: index.ProjectionStateReady, Basis: "c1", ObjectCount: 10,
	}
	var mu sync.Mutex
	controlWrites := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/_doc/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"_source": old, "_seq_no": 7, "_primary_term": 1})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "-g-"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_doc/"):
			mu.Lock()
			controlWrites++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"_seq_no": 8, "_primary_term": 1})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "-g-"):
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, r.Method+" "+r.URL.String(), http.StatusNotFound)
		}
	}))
	defer server.Close()
	engine := &openSearchEngine{
		base: server.URL, http: server.Client(), prefix: "kc-proj-test", controlID: "control",
		repository: "kr://acme/public/core", primaryShards: 8, refreshInterval: "1s",
	}
	session, err := engine.BeginRebuild(index.Meta{Basis: "c2"})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		meta, loadErr := engine.LoadMeta()
		if loadErr == nil && (meta.State != index.ProjectionStateReady || meta.Basis != "c1") {
			loadErr = fmt.Errorf("warm rebuild exposed %#v", meta)
		}
		result <- loadErr
	}()
	select {
	case loadErr := <-result:
		if loadErr != nil {
			t.Fatal(loadErr)
		}
	case <-time.After(time.Second):
		t.Fatal("warm rebuild blocked the old READY generation")
	}
	mu.Lock()
	writes := controlWrites
	mu.Unlock()
	if writes != 0 {
		t.Fatalf("warm rebuild must not publish BUILDING over READY, writes=%d", writes)
	}
	if err := session.Abort(errors.New("test abort")); err == nil {
		t.Fatal("abort must return its cause")
	}
}

func TestOpenSearchIncrementalApplyAvoidsGlobalCountAndForcedRefresh(t *testing.T) {
	control := controlDoc{
		Repository: "kr://acme/public/core", ActiveIndex: "kc-proj-test-g-old", Generation: "old",
		State: index.ProjectionStateReady, Basis: "c1", ObjectCount: 10,
	}
	var final controlDoc
	var forbidden []string
	var bulkWaitFor bool
	seq := int64(7)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/_doc/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"_source": control, "_seq_no": seq, "_primary_term": 1})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_doc/"):
			if err := json.NewDecoder(r.Body).Decode(&final); err != nil {
				t.Error(err)
			}
			seq++
			_ = json.NewEncoder(w).Encode(map[string]any{"_seq_no": seq, "_primary_term": 1})
		case r.Method == http.MethodPost && r.URL.Path == "/_bulk":
			bulkWaitFor = r.URL.Query().Get("refresh") == "wait_for"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errors": false,
				"items":  []any{map[string]any{"index": map[string]any{"status": 201, "result": "created"}}},
			})
		case strings.HasSuffix(r.URL.Path, "/_count") || strings.HasSuffix(r.URL.Path, "/_refresh"):
			forbidden = append(forbidden, r.URL.Path)
			http.Error(w, "forbidden full-index operation", http.StatusInternalServerError)
		default:
			http.Error(w, r.Method+" "+r.URL.String(), http.StatusNotFound)
		}
	}))
	defer server.Close()
	engine := &openSearchEngine{
		base: server.URL, http: server.Client(), prefix: "kc-proj-test", controlID: "control",
		repository: "kr://acme/public/core",
	}
	err := engine.Apply([]index.CompiledDoc{{ObjectID: "policy/P-11"}}, nil, index.Meta{Basis: "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(forbidden) != 0 || !bulkWaitFor {
		t.Fatalf("forbidden=%v refresh_wait_for=%v", forbidden, bulkWaitFor)
	}
	if final.State != index.ProjectionStateReady || final.ObjectCount != 11 || final.Basis != "c2" {
		t.Fatalf("final control: %#v", final)
	}
}
