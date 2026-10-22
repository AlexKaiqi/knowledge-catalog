package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

func TestResourceRecoveryRejectsInvalidGenerationBeforePhysicalDeletion(t *testing.T) {
	for _, generation := range []string{"*,other-index", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("z", 32)} {
		t.Run(generation, func(t *testing.T) {
			var records resourceControlFixture
			deletes := 0
			record := controlDoc{State: "RESOURCE", Repository: "repo", Generation: generation, ActiveIndex: "kc-test-g-" + generation,
				Resource: &generationResource{Version: 1, Phase: resourceBuilding, LeaseUntil: time.Now().Add(-time.Minute).UnixMilli()}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletes++
					fmt.Fprint(w, `{}`)
					return
				}
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_search") {
					_ = json.NewEncoder(w).Encode(map[string]any{"_shards": map[string]any{"total": 1, "successful": 1, "failed": 0}, "hits": map[string]any{"hits": []any{map[string]any{"_id": "test-resource-" + generation, "_source": record}}}})
					return
				}
				if !records.serve(w, r) {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			e := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			if _, err := e.putControlRecord("test-resource-"+generation, record, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := e.RecoverResources(context.Background()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || deletes != 0 {
				t.Fatalf("malformed generation became a physical delete target: %v, deletes=%d", err, deletes)
			}
		})
	}
}

func TestResourceRecoveryPersistsProgressBeyondUnresolvedAdmissions(t *testing.T) {
	const total = 2050
	var records resourceControlFixture
	firstAfterReopen := ""
	reopened := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/"+controlIndexName+"/_search" {
			var request struct {
				Size  int      `json:"size"`
				After []string `json:"search_after"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			start := 0
			if len(request.After) > 0 {
				start, _ = strconv.Atoi(request.After[0])
				if reopened && firstAfterReopen == "" {
					firstAfterReopen = request.After[0]
				}
			}
			hits := []any{}
			for number := start + 1; number <= total && len(hits) < request.Size; number++ {
				generation := fmt.Sprintf("%032d", number)
				hits = append(hits, map[string]any{
					"_id": "test-resource-" + generation, "sort": []string{generation},
					"_source": controlDoc{Repository: "repo", ActiveIndex: "kc-test-g-" + generation, Generation: generation},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"_shards": map[string]any{"total": 1, "successful": 1, "failed": 0}, "hits": map[string]any{"hits": hits}})
			return
		}
		if !records.serve(w, r) {
			t.Errorf("unexpected physical request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected physical request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "test", repository: "repo", prefix: "kc-test"}
	first, err := engine.RecoverResources(context.Background())
	if err != nil || !first.More || len(first.Retained) == 0 || len(first.Retained) >= total || len(first.Deleted) != 0 {
		t.Fatalf("bounded pass lost unresolved protection or claimed completion: %#v %v", first, err)
	}
	reopened = true
	fresh := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "test", repository: "repo", prefix: "kc-test"}
	last, err := fresh.RecoverResources(context.Background())
	if err != nil || last.More || len(first.Retained)+len(last.Retained) != total || firstAfterReopen == "" {
		t.Fatalf("restart starved work beyond the bounded pass: first=%d last=%#v after=%s err=%v", len(first.Retained), last, firstAfterReopen, err)
	}
	cursor, _, err := fresh.loadControlRecord(context.Background(), "test-resource-recovery")
	if err != nil || cursor.RecoveryAfter != "" {
		t.Fatalf("finished pass did not reset durable cursor: %#v %v", cursor, err)
	}
}

func TestLegacyNamespaceCannotAdmitHistoricalDeletion(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "legacy namespace has no exclusive owner", 500)
	}))
	defer server.Close()
	e := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
	session := &rebuildSession{engine: e, generation: strings.Repeat("a", 32), physicalIndex: "kc-test-g-" + strings.Repeat("a", 32)}
	if err := session.discardHistoricalResource(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || calls != 0 {
		t.Fatalf("legacy namespace entered historical reclamation: calls=%d %v", calls, err)
	}
}
