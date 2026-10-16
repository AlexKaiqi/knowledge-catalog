package opensearchimpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
)

func recoveringControlBody(index, state string) string {
	return fmt.Sprintf(`{"error":{"type":"no_shard_available_action_exception","caused_by":{"type":"illegal_index_shard_state_exception","index":%q,"reason":%q}},"status":503}`, index, "CurrentState["+state+"] operations only allowed when shard state is one of [POST_RECOVERY, STARTED]")
}

func TestControlReadWaitsForRecoveringPrimaryWithoutCreatingAnything(t *testing.T) {
	var reads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/"+controlIndexName+"/_doc/first-read" {
			t.Errorf("metadata readiness changed operation: %s %s", r.Method, r.URL)
		}
		if reads.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, recoveringControlBody(controlIndexName, "RECOVERING"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"found":false}`)
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "first-read"}
	_, version, err := engine.loadControlContext(context.Background())
	if err != nil || version != nil || reads.Load() != 2 {
		t.Fatalf("recovering primary was mistaken for unavailable data: reads=%d version=%#v error=%v", reads.Load(), version, err)
	}
}

func TestControlReadDoesNotRetryOtherProviderFailures(t *testing.T) {
	for _, test := range []struct{ name, body string }{{"arbitrary-503", `{"error":"unavailable"}`}, {"other-index", recoveringControlBody("foreign", "RECOVERING")}, {"other-state", recoveringControlBody(controlIndexName, "CLOSED")}} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			if _, _, err := engine.loadControlContext(context.Background()); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || requests.Load() != 1 {
				t.Fatalf("unrelated failure was retried or hidden: %d %v", requests.Load(), err)
			}
		})
	}
}

func TestControlRecoveryWaitRetainsDeadlineAndStopsOnOtherErrors(t *testing.T) {
	for _, mode := range []string{"recovering", "blocked", "other-error"} {
		t.Run(mode, func(t *testing.T) {
			var reads atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := reads.Add(1)
				if n > 1 && mode == "blocked" {
					<-r.Context().Done()
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				if n > 1 && mode == "other-error" {
					fmt.Fprint(w, `{"error":"unavailable"}`)
					return
				}
				fmt.Fprint(w, recoveringControlBody(controlIndexName, "RECOVERING"))
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client()}
			ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
			defer cancel()
			started := time.Now()
			if _, _, err := engine.loadControlContext(ctx); err == nil || time.Since(started) > 500*time.Millisecond || reads.Load() < 2 {
				t.Fatalf("failed recovery escaped its boundary: reads=%d error=%v", reads.Load(), err)
			}
			if mode == "other-error" && reads.Load() != 2 {
				t.Fatalf("unrelated later failure was retried: %d", reads.Load())
			}
		})
	}
}

func TestControlRecoveryHasItsOwnFiniteWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, recoveringControlBody(controlIndexName, "RECOVERING"))
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := engine.loadControlContext(ctx); done <- err }()
	select {
	case err := <-done:
		if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
			t.Fatalf("unrecovered primary lost typed unavailable result: %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		cancel()
		<-done
		t.Fatal("control recovery inherited an unbounded caller context")
	}
}
