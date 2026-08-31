package llmhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/retrieval/llmhttp"
)

func providerRequest() retrieval.RerankRequest {
	return retrieval.RerankRequest{
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:test", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "refund timeout relevance", OutputContract: retrieval.OutputContract{},
		},
		Candidates: []retrieval.Candidate{
			{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/core", Object: "p1"}, Value: map[string]any{"body": "deploy"}},
			{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/core", Object: "p2"}, Value: map[string]any{"body": "refund timeout"}},
		},
	}
}

func TestProviderUsesOneNonReasoningStructuredResponsesCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-5.6-luna" || body["reasoning"].(map[string]any)["effort"] != "none" {
			t.Fatalf("model/reasoning = %#v", body)
		}
		if _, unsupported := body["max_output_tokens"]; unsupported {
			t.Fatalf("endpoint-incompatible max_output_tokens was sent: %#v", body)
		}
		format := body["text"].(map[string]any)["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Fatalf("format = %#v", format)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.6-luna","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"groups\":[{\"candidateIds\":[\"candidate_2\"]},{\"candidateIds\":[\"candidate_1\"]}],\"unjudged\":[]}"}]}]}`))
	}))
	defer server.Close()
	provider, err := llmhttp.New(llmhttp.Config{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "gpt-5.6-luna", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Rerank(context.Background(), providerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Groups[0].Refs[0].Object != "p2" || result.Provider != "llm-native" || result.ModelRevision != "gpt-5.6-luna" {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
}

func TestProviderNormalizesTransientAndInvalidOutputFailures(t *testing.T) {
	for name, test := range map[string]struct {
		status int
		body   string
		code   kernel.ErrorCode
	}{
		"rate-limit": {http.StatusTooManyRequests, `{}`, kernel.ErrTemporaryUnavailable},
		"bad-output": {http.StatusOK, `{"model":"gpt-5.6-luna","output":[]}`, kernel.ErrPreconditionFailed},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			provider, err := llmhttp.New(llmhttp.Config{BaseURL: server.URL, APIKey: "secret", Model: "gpt-5.6-luna", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Rerank(context.Background(), providerRequest()); kernel.CodeOf(err) != test.code {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
