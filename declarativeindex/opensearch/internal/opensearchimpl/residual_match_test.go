package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	retrieval "kc/discovery"
)

type contextualResidualVerifier interface {
	VerifyResidualMatchContext(context.Context, string, string, retrieval.MatchMode) (bool, error)
}

func TestOpenSearchResidualMatchUsesPublishedAnalyzerAndPositions(t *testing.T) {
	responses := map[string]string{
		"punctuated": `[{"token":"healthy","position":0},{"token":"orders","position":1}]`,
		"query":      `[{"token":"healthy","position":0},{"token":"orders","position":1}]`,
		"reversed":   `[{"token":"orders","position":0},{"token":"healthy","position":1}]`,
		"gap":        `[{"token":"healthy","position":0},{"token":"orders","position":2}]`,
		"one":        `[{"token":"healthy","position":0}]`,
		"other":      `[{"token":"unhealthy","position":0}]`,
		"empty":      `[]`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/"+controlIndexName+"/_doc/owned" {
			fmt.Fprint(w, `{"_source":{"active_index":"owned-g-one","generation":"one","state":"READY"}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/owned-g-one/_analyze" {
			t.Errorf("residual verification changed provider data: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", 400)
			return
		}
		var body struct{ Field, Text string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Field != "all_text" {
			t.Errorf("verification must use the actual projection field analyzer: %#v %v", body, err)
		}
		fmt.Fprintf(w, `{"tokens":%s}`, responses[body.Text])
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), prefix: "owned", controlID: "owned"}
	verifier, ok := any(engine).(contextualResidualVerifier)
	if !ok {
		t.Fatal("OpenSearch cannot verify a freshly hydrated MATCH candidate")
	}
	for _, tc := range []struct {
		text, query string
		mode        retrieval.MatchMode
		want        bool
	}{
		{"punctuated", "query", retrieval.MatchAllTerms, true},
		{"reversed", "query", retrieval.MatchAllTerms, true},
		{"reversed", "query", retrieval.MatchPhrase, false},
		{"punctuated", "query", retrieval.MatchPhrase, true},
		{"gap", "query", retrieval.MatchPhrase, false},
		{"gap", "gap", retrieval.MatchPhrase, true},
		{"one", "query", retrieval.MatchAnyTerms, true},
		{"one", "query", retrieval.MatchAllTerms, false},
		{"other", "one", retrieval.MatchAllTerms, false},
		{"punctuated", "empty", retrieval.MatchAllTerms, false},
	} {
		t.Run(tc.text+"/"+tc.query+"/"+string(tc.mode), func(t *testing.T) {
			got, err := verifier.VerifyResidualMatchContext(context.Background(), tc.text, tc.query, tc.mode)
			if err != nil || got != tc.want {
				t.Fatalf("provider analyzed match: got=%v want=%v error=%v", got, tc.want, err)
			}
		})
	}
}

func TestOpenSearchResidualMatchFailsClosedAndKeepsCancellation(t *testing.T) {
	for _, response := range []string{`{}`, `{"tokens":null}`, `{"tokens":[{"token":"healthy"}]}`, `{"tokens":[{"token":"healthy","position":-1}]}`, `{"tokens":[{"token":"healthy","position":0,"positionLength":2}]}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					fmt.Fprint(w, `{"_source":{"active_index":"owned-g-one","state":"READY"}}`)
				} else {
					fmt.Fprint(w, response)
				}
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client(), prefix: "owned"}
			verifier, ok := any(engine).(contextualResidualVerifier)
			if !ok {
				t.Fatal("missing analyzed residual verifier")
			}
			if _, err := verifier.VerifyResidualMatchContext(context.Background(), "healthy", "healthy", retrieval.MatchAllTerms); err == nil {
				t.Fatal("invalid analysis was mistaken for a match or zero matches")
			}
		})
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"_source":{"active_index":"owned-g-one","state":"READY"}}`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), prefix: "owned"}
	verifier, ok := any(engine).(contextualResidualVerifier)
	if !ok {
		t.Fatal("missing context-aware residual verifier")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifier.VerifyResidualMatchContext(ctx, "healthy", "healthy", retrieval.MatchAllTerms); err == nil || requests.Load() != 0 {
		t.Fatalf("pre-cancelled verification reached the provider: requests=%d error=%v", requests.Load(), err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := verifier.VerifyResidualMatchContext(ctx, "healthy", "healthy", retrieval.MatchAllTerms); err == nil || time.Since(started) > time.Second {
		t.Fatalf("verification escaped the request deadline: %v", err)
	}
}
