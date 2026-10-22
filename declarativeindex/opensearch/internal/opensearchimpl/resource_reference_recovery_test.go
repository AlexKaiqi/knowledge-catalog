package opensearchimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPublicationRecoveryPersistsBoundedProgressPastUnknownAuthorities(t *testing.T) {
	for _, phase := range []string{claimAdmitted, claimAccepted} {
		t.Run(phase, func(t *testing.T) {
			const total = 2050
			var records resourceControlFixture
			reopened := false
			firstAfterReopen := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_search") {
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
						if len(request.After) != 2 {
							t.Error("lost repository/generation cursor")
							return
						}
						start, _ = strconv.Atoi(request.After[1])
						if reopened && firstAfterReopen == "" {
							firstAfterReopen = request.After[1]
						}
					}
					hits := []any{}
					for number := start + 1; number <= total && len(hits) < request.Size; number++ {
						generation := fmt.Sprintf("%032d", number)
						hits = append(hits, map[string]any{"_id": "test-resource-" + generation, "sort": []string{"repo", generation}, "_source": controlDoc{
							State: "RESOURCE", Repository: "repo", Generation: generation, ActiveIndex: "kc-test-g-" + generation,
							Resource: &generationResource{Version: resourceRecordVersion, Phase: resourceAdopted, LeaseUntil: 1,
								Claims: []resourceClaim{{ID: generation, Phase: phase, Until: 1, Evidence: []byte("unknown-authority")}}},
						}})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"_shards": map[string]any{"total": 1, "successful": 1, "failed": 0}, "hits": map[string]any{"hits": hits}})
					return
				}
				if !records.serve(w, r) {
					t.Errorf("unexpected physical IO: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			unknown := errors.New("authority unavailable")
			resolve := func(context.Context, []byte) (bool, error) { return false, unknown }
			run := engine.RecoverPublications
			if phase == claimAccepted {
				run = engine.ReconcilePublicationReferences
			}
			first, err := run(context.Background(), resolve)
			if !errors.Is(err, unknown) || !first.More || len(first.Retained) != 2048 || len(first.Released) != 0 {
				t.Fatalf("unresolved authorities lost bounded progress: count=%d more=%t err=%v", len(first.Retained), first.More, err)
			}
			reopened = true
			fresh := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			run = fresh.RecoverPublications
			if phase == claimAccepted {
				run = fresh.ReconcilePublicationReferences
			}
			last, err := run(context.Background(), resolve)
			if !errors.Is(err, unknown) || last.More || len(last.Retained) != 2 || firstAfterReopen == "" {
				t.Fatalf("reopen starved tail references: count=%d more=%t after=%s err=%v", len(last.Retained), last.More, firstAfterReopen, err)
			}
			cursorID := "test-publication-recovery"
			if phase == claimAccepted {
				cursorID += "-references"
			}
			cursor, _, err := fresh.loadControlRecord(context.Background(), cursorID)
			if err != nil || cursor.RecoveryAfter != "" {
				t.Fatalf("finished pass did not reset durable cursor: %#v %v", cursor, err)
			}
		})
	}
}

func TestPublicationRecoveryRejectsIncompleteAuthorityEnumeration(t *testing.T) {
	for _, response := range []string{
		`{"timed_out":true,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[]}}`,
		`{"_shards":{"total":2,"successful":1,"failed":1},"hits":{"hits":[]}}`,
		`{"hits":{"hits":[]}}`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					http.NotFound(w, r)
					return
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected mutation: %s", r.Method)
				}
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			e := &openSearchEngine{base: server.URL, http: server.Client(), repository: "repo", prefix: "kc-test", controlID: "test"}
			calls := 0
			report, err := e.RecoverPublications(context.Background(), func(context.Context, []byte) (bool, error) { calls++; return false, nil })
			if err == nil || calls != 0 || len(report.Released) != 0 {
				t.Fatalf("partial enumeration produced completion: %#v calls=%d err=%v", report, calls, err)
			}
		})
	}
}

func TestResourceCoordinatorConstructionDoesNotProbeBackendOrOwnLegacyNamespace(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "construction must stay local", 500)
	}))
	defer server.Close()
	manager, err := OpenResources(Config{URL: server.URL, Namespace: "owned", PrimaryShards: 1})
	if err != nil || manager == nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := OpenResources(Config{URL: server.URL, PrimaryShards: 1})
	if err != nil || legacy != nil || calls != 0 {
		t.Fatalf("local construction adopted legacy resources or probed backend: manager=%v calls=%d err=%v", legacy, calls, err)
	}
}
