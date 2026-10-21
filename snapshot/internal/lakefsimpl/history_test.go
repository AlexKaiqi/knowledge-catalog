package lakefsimpl

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

	"kc/kernel"
)

func historyRepository(t *testing.T, serve http.HandlerFunc) *Repository {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/c02504") {
			_, _ = w.Write([]byte(`{"id":"c02504"}`))
			return
		}
		serve(w, r)
	}))
	t.Cleanup(server.Close)
	ep := Endpoint{API: server.URL + "/api/v1", Repository: "history"}
	client, err := newAPIClient(ep, "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	return &Repository{id: "kr://test/history", ep: ep, client: client, repoState: &repoState{}}
}

func TestCommitHistoryFollowsLakeFSPagesToRequestedLimit(t *testing.T) {
	var requests []string
	repo := historyRepository(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		if r.URL.Query().Get("first_parent") != "true" {
			t.Error("history must follow first parent")
		}
		amount, _ := strconv.Atoi(r.URL.Query().Get("amount"))
		if amount < 1 || amount > 1000 {
			t.Errorf("invalid page amount %d", amount)
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("after"))
		end := offset + amount
		if end > 2505 {
			end = 2505
		}
		results := []map[string]string{}
		for i := offset; i < end; i++ {
			results = append(results, map[string]string{"id": fmt.Sprintf("c%05d", 2504-i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "pagination": map[string]any{"has_more": end < 2505, "next_offset": strconv.Itoa(end)}})
	})
	for _, limit := range []int{2100, 3000} {
		requests = nil
		got, err := repo.CommitHistory("c02504", limit)
		if err != nil {
			t.Fatal(err)
		}
		want := limit
		if want > 2505 {
			want = 2505
		}
		if len(got) != want {
			t.Fatalf("limit %d: got %d commits in %d requests, want %d", limit, len(got), len(requests), want)
		}
		for i, id := range got {
			if id != kernel.CommitID(fmt.Sprintf("c%05d", 2504-i)) {
				t.Fatalf("commit %d = %s", i, id)
			}
		}
		if len(requests) != 3 {
			t.Fatalf("expected three bounded pages, got %v", requests)
		}
	}
}

func TestCommitHistoryRejectsBrokenPaginationAndLaterErrors(t *testing.T) {
	for _, mode := range []string{"missing-offset", "repeated-offset", "cycle", "later-error"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			repo := historyRepository(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 5 {
					t.Error("history pagination did not terminate")
					http.Error(w, "loop", http.StatusInternalServerError)
					return
				}
				if mode == "later-error" && calls == 2 {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				next := "next"
				if mode == "missing-offset" {
					next = ""
				}
				if mode == "cycle" {
					if calls%2 == 0 {
						next = "other"
					} else {
						next = "next"
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{"id": fmt.Sprintf("c%d", calls)}}, "pagination": map[string]any{"has_more": true, "next_offset": next}})
			})
			got, err := repo.CommitHistory("c02504", 100)
			if err == nil || len(got) != 0 {
				t.Fatalf("%s: partial history = %#v, %v", mode, got, err)
			}
			if mode != "later-error" && kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
				t.Fatalf("broken pagination = %v", err)
			}
		})
	}
}

func TestCommitHistoryCancellationOnLaterPageDoesNotReturnPartialHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	repo := historyRepository(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			cancel()
			<-r.Context().Done()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{"id": "c02504"}}, "pagination": map[string]any{"has_more": true, "next_offset": "older"}})
	})
	bound := repo.WithContext(ctx)
	got, err := bound.CommitHistory("c02504", 100)
	if !errors.Is(err, context.Canceled) || len(got) != 0 {
		t.Fatalf("canceled history = %#v, %v", got, err)
	}
}
