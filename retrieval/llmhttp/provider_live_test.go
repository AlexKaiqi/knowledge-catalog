package llmhttp_test

import (
	"context"
	"os"
	"testing"
	"time"

	"kc/knowledge"
	"kc/retrieval"
	"kc/retrieval/llmhttp"
)

func TestLiveLunaListwiseRerank(t *testing.T) {
	if os.Getenv("KC_LIVE_LLM_RERANK") != "1" {
		t.Skip("set KC_LIVE_LLM_RERANK=1 for paid live LLM validation")
	}
	provider, err := llmhttp.New(llmhttp.Config{
		BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"),
		Model: "gpt-5.6-luna", Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := retrieval.RerankRequest{
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:live-smoke", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion:            "Rank by usefulness for diagnosing a customer refund request that times out. Prefer directly actionable diagnosis over unrelated operational material.",
			EvaluationProjection: &retrieval.EvaluationProjection{Fields: []string{"body"}},
			OutputContract:       retrieval.OutputContract{AllowUnjudged: false},
		},
		Candidates: []retrieval.Candidate{
			{Ref: knowledge.KnowledgeRef{Repository: "kr://live/synthetic", Object: "deployment"}, Value: map[string]any{"body": "Kubernetes deployment rollout checklist", "secret": "must-not-leak"}},
			{Ref: knowledge.KnowledgeRef{Repository: "kr://live/synthetic", Object: "refund-timeout"}, Value: map[string]any{"body": "Check payment gateway timeout logs, refund idempotency key, and retry status before resubmitting", "secret": "must-not-leak"}},
			{Ref: knowledge.KnowledgeRef{Repository: "kr://live/synthetic", Object: "office"}, Value: map[string]any{"body": "Office visitor registration process", "secret": "must-not-leak"}},
		},
	}
	result, err := retrieval.ExecuteRerank(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) == 0 || len(result.Groups[0].Refs) == 0 || result.Groups[0].Refs[0].Object != "refund-timeout" {
		t.Fatalf("unexpected live ranking: %#v", result.Groups)
	}
	if result.Evidence == nil || result.Evidence.Provider != "llm-native" || result.Evidence.Model != "gpt-5.6-luna" {
		t.Fatalf("missing live evidence: %#v", result.Evidence)
	}
}
