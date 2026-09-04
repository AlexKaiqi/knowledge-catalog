package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
	"kc/retrieval"
	"kc/retrieval/llmhttp"
)

type recordingReranker struct {
	request retrieval.RerankRequest
	calls   int
	err     error
	result  *retrieval.RerankProviderResult
}

func (r *recordingReranker) Rerank(_ context.Context, request retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
	r.calls++
	r.request = request
	if r.err != nil {
		return retrieval.RerankProviderResult{}, r.err
	}
	if r.result != nil {
		return *r.result, nil
	}
	refs := make([]knowledge.KnowledgeRef, len(request.Candidates))
	for i, candidate := range request.Candidates {
		refs[i] = candidate.Ref
	}
	return retrieval.RerankProviderResult{
		Groups: []retrieval.RankGroup{
			{Rank: 1, Refs: []knowledge.KnowledgeRef{refs[1]}},
			{Rank: 2, Refs: []knowledge.KnowledgeRef{refs[0]}},
		},
		Provider: "llm-runtime", Model: "listwise-judge", ModelRevision: "2026-08-31",
	}, nil
}

func rerankHTTP(t *testing.T, server *httptest.Server, body map[string]any) (int, map[string]any) {
	return rerankHTTPAs(t, server, "agent:rerank-test", body)
}

func rerankHTTPAs(t *testing.T, server *httptest.Server, principal string, body map[string]any) (int, map[string]any) {
	return semanticHTTPAs(t, server, "/knowledge/v1/rerank", principal, body)
}

func semanticHTTPAs(t *testing.T, server *httptest.Server, path, principal string, body map[string]any) (int, map[string]any) {
	return semanticHTTPWithTrace(t, server, path, principal, "", body)
}

func semanticHTTPWithTrace(t *testing.T, server *httptest.Server, path, principal, traceID string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", principal)
	if traceID != "" {
		request.Header.Set("X-Kc-Trace-Id", traceID)
		request.Header.Set("X-Kc-Span-Id", "span-rerank")
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, payload
}

func TestRerankEvidenceFeedbackAndTrainingSampleJourney(t *testing.T) {
	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repository := "kr://acme/public/training"
	workspace := "agent"
	agent := "agent:answer"
	user := "user:kai"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "training-p1", "--repo", repository,
		"--object", "runbook/deploy", "--value", `{"body":"deployment checklist","secret":"never-record"}`))
	body(t, kc(home, "put", "--command-id", "training-p2", "--repo", repository,
		"--object", "runbook/refund", "--value", `{"body":"refund timeout diagnosis","secret":"never-record"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	for _, principal := range []string{agent, user} {
		body(t, kc(home, "allow", "--principal", principal, "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
		body(t, kc(home, "allow", "--principal", principal, "--action", "knowledge.read,knowledge.rerank", "--repo", repository))
		body(t, kc(home, "allow", "--principal", principal, "--action", "feedback.write", "--catalog", catalog, "--workspace", workspace))
		body(t, kc(home, "allow", "--principal", principal, "--action", "audit.read", "--catalog", catalog))
	}
	provider := &recordingReranker{}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Reranker: provider})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	traceID := "trace-training"
	status, response := semanticHTTPWithTrace(t, server, "/knowledge/v1/rerank", agent, traceID, map[string]any{
		"catalog": catalog, "workspace": workspace,
		"candidates": []any{
			map[string]any{"repository": repository, "object": "runbook/deploy"},
			map[string]any{"repository": repository, "object": "runbook/refund"},
		},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:training", "revision": 1, "operator": "SEMANTIC_RERANK",
			"criterion": "refund timeout relevance", "evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract": map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("rerank status=%d response=%#v", status, response)
	}
	refineID := asMap(t, response["evidence"])["refineEvidenceId"].(string)
	if !strings.HasPrefix(refineID, "rf_") {
		t.Fatalf("refine evidence id = %q", refineID)
	}
	rawEvidence, err := os.ReadFile(filepath.Join(home, "refine.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rawEvidence, []byte("never-record")) || !bytes.Contains(rawEvidence, []byte("refund timeout diagnosis")) || !bytes.Contains(rawEvidence, []byte("refund timeout relevance")) {
		t.Fatalf("refine evidence projection boundary violated: %s", rawEvidence)
	}
	selected := []any{map[string]any{"repository": repository, "object": "runbook/refund"}}
	status, answered := semanticHTTPAs(t, server, "/operations/v1/feedback", agent, map[string]any{
		"workspace": workspace, "traceId": traceID, "outcome": "answered", "refineEvidenceId": refineID,
		"answer": "Inspect the refund gateway timeout and idempotency status.", "selectedRefs": selected,
	})
	if status != http.StatusOK || answered["recorded"] != true {
		t.Fatalf("answered feedback: status=%d payload=%#v", status, answered)
	}
	status, accepted := semanticHTTPAs(t, server, "/operations/v1/feedback", user, map[string]any{
		"workspace": workspace, "traceId": traceID, "outcome": "accepted", "refineEvidenceId": refineID,
	})
	if status != http.StatusOK || accepted["recorded"] != true {
		t.Fatalf("accepted feedback: status=%d payload=%#v", status, accepted)
	}
	status, log := semanticHTTPAs(t, server, "/operations/v1/refine-log:query", agent, map[string]any{"evidenceId": refineID})
	if status != http.StatusOK || len(log["entries"].([]any)) != 1 {
		t.Fatalf("refine log: status=%d payload=%#v", status, log)
	}
	status, training := semanticHTTPAs(t, server, "/operations/v1/rerank-training:query", agent, map[string]any{"evidenceId": refineID})
	if status != http.StatusOK {
		t.Fatalf("training query: status=%d payload=%#v", status, training)
	}
	samples := training["samples"].([]any)
	if len(samples) != 1 || asMap(t, samples[0])["trainingEligible"] != true || asMap(t, samples[0])["labelStrength"] != "accepted-answer" {
		t.Fatalf("training samples = %#v", samples)
	}
	feedback := asMap(t, samples[0])["feedback"].([]any)
	firstFeedback := asMap(t, feedback[0])
	if asMap(t, firstFeedback["trace"])["traceId"] != traceID || asMap(t, firstFeedback["submissionTrace"])["traceId"] == traceID {
		t.Fatalf("feedback target/submission trace separation = %#v", firstFeedback)
	}
	provider.err = kernel.Fail(kernel.ErrTemporaryUnavailable, "synthetic model timeout")
	status, failed := semanticHTTPWithTrace(t, server, "/knowledge/v1/rerank", agent, traceID, map[string]any{
		"catalog": catalog, "workspace": workspace,
		"candidates": []any{
			map[string]any{"repository": repository, "object": "runbook/deploy"},
			map[string]any{"repository": repository, "object": "runbook/refund"},
		},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:training", "revision": 1, "operator": "SEMANTIC_RERANK",
			"criterion": "refund timeout relevance", "evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract": map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusServiceUnavailable || asMap(t, failed["error"])["code"] != "TEMPORARY_UNAVAILABLE" {
		t.Fatalf("failed rerank: status=%d payload=%#v", status, failed)
	}
	status, log = semanticHTTPAs(t, server, "/operations/v1/refine-log:query", agent, map[string]any{"traceId": traceID})
	entries := log["entries"].([]any)
	if status != http.StatusOK || len(entries) != 2 || asMap(t, entries[1])["outcome"] != "ERROR" || asMap(t, entries[1])["error"] == nil {
		t.Fatalf("failed refine evidence: status=%d payload=%#v", status, log)
	}
	provider.err = nil
	provider.result = &retrieval.RerankProviderResult{
		Provider: "llm-runtime", Model: "listwise-judge",
		Groups: []retrieval.RankGroup{{Rank: 1, Refs: []knowledge.KnowledgeRef{{Repository: kernel.RepositoryID(repository), Object: "runbook/not-a-candidate"}}}},
	}
	status, invalid := semanticHTTPWithTrace(t, server, "/knowledge/v1/rerank", agent, traceID, map[string]any{
		"catalog": catalog, "workspace": workspace,
		"candidates": []any{
			map[string]any{"repository": repository, "object": "runbook/deploy"},
			map[string]any{"repository": repository, "object": "runbook/refund"},
		},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:training", "revision": 1, "operator": "SEMANTIC_RERANK",
			"criterion": "refund timeout relevance", "evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract": map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusConflict || asMap(t, invalid["error"])["code"] != "PRECONDITION_FAILED" {
		t.Fatalf("invalid provider output: status=%d payload=%#v", status, invalid)
	}
	status, log = semanticHTTPAs(t, server, "/operations/v1/refine-log:query", agent, map[string]any{
		"traceId": traceID, "provider": "llm-runtime", "model": "listwise-judge", "outcome": "ERROR",
	})
	entries = log["entries"].([]any)
	if status != http.StatusOK || len(entries) != 1 || asMap(t, entries[0])["modelOutput"] == nil {
		t.Fatalf("invalid model output evidence: status=%d payload=%#v", status, log)
	}
}

func TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repository := "kr://acme/public/search-rerank"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "search-rerank-schema", "--repo", repository,
		"--object", "schema/runbook.search-rerank", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(home, "put", "--command-id", "search-rerank-p1", "--repo", repository,
		"--object", "runbook/deploy", "--schema-ref", "schema/runbook.search-rerank", "--value", `{"body":"refund deployment checklist","secret":"one"}`))
	body(t, kc(home, "put", "--command-id", "search-rerank-p2", "--repo", repository,
		"--object", "runbook/timeout", "--schema-ref", "schema/runbook.search-rerank", "--value", `{"body":"refund timeout diagnosis","secret":"two"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--action", "knowledge.read,knowledge.search,knowledge.rerank", "--repo", repository))
	syncIndexes(t, home, repository)

	provider := &recordingReranker{}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Reranker: provider})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status, response := semanticHTTPAs(t, server, "/knowledge/v1/search:rerank", "agent:rerank-test", map[string]any{
		"catalog": catalog, "workspace": workspace, "query": "refund", "limit": 2,
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:search-rerank", "revision": 1, "operator": "SEMANTIC_RERANK",
			"criterion": "refund timeout diagnosis", "evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract": map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusOK || provider.calls != 1 {
		t.Fatalf("status=%d response=%#v calls=%d", status, response, provider.calls)
	}
	retrievalStage := asMap(t, response["retrieval"])
	rerankStage := asMap(t, response["rerank"])
	if len(retrievalStage["hits"].([]any)) != 2 || retrievalStage["searchView"] == nil || rerankStage["searchView"] == nil {
		t.Fatalf("two-stage evidence = %#v", response)
	}
	retrievalID, _ := retrievalStage["retrievalEvidenceId"].(string)
	refineID, _ := asMap(t, rerankStage["evidence"])["refineEvidenceId"].(string)
	if !strings.HasPrefix(retrievalID, "rt_") || !strings.HasPrefix(refineID, "rf_") {
		t.Fatalf("retrieval/refine evidence ids = %q / %q", retrievalID, refineID)
	}
	store := observability.NewFileStore(home)
	retrievalEvents, err := store.Retrieval(observability.RetrievalQuery{EvidenceID: retrievalID})
	if err != nil || len(retrievalEvents) != 1 || retrievalEvents[0].Operator != observability.RetrievalOperatorSearch || len(retrievalEvents[0].Candidates) != 2 {
		t.Fatalf("retrieval evidence = %#v err=%v", retrievalEvents, err)
	}
	refineEvents, err := store.Refine(observability.RefineQuery{EvidenceID: refineID})
	if err != nil || len(refineEvents) != 1 || refineEvents[0].RetrievalEvidenceID != retrievalID {
		t.Fatalf("refine retrieval linkage = %#v err=%v", refineEvents, err)
	}
	if !reflect.DeepEqual(retrievalStage["searchView"], rerankStage["searchView"]) {
		t.Fatalf("rerank changed SearchView: retrieval=%#v rerank=%#v", retrievalStage["searchView"], rerankStage["searchView"])
	}
	evidence := asMap(t, rerankStage["evidence"])
	candidates := evidence["candidates"].([]any)
	if len(candidates) != 2 || asMap(t, candidates[0])["originalRank"] != float64(1) {
		t.Fatalf("candidate evidence = %#v", candidates)
	}
	laneEvidence := asMap(t, candidates[0])["retrievalEvidence"].([]any)
	if len(laneEvidence) == 0 || asMap(t, laneEvidence[0])["lane"] == "" || asMap(t, laneEvidence[0])["provider"] == "" {
		t.Fatalf("retrieval provenance = %#v", laneEvidence)
	}
	if len(provider.request.Candidates) != 2 || provider.request.Candidates[0].OriginalRank != 0 || len(provider.request.Candidates[0].RetrievalEvidence) != 0 {
		t.Fatalf("physical rank leaked to model request: %#v", provider.request.Candidates)
	}
}

func TestLiveHTTPSearchRerankWithLuna(t *testing.T) {
	if os.Getenv("KC_LIVE_LLM_RERANK") != "1" {
		t.Skip("set KC_LIVE_LLM_RERANK=1 for paid live LLM validation")
	}
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required")
	}
	provider, err := llmhttp.New(llmhttp.Config{
		BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"),
		Model: "gpt-5.6-luna", Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repository := "kr://acme/public/live-search-rerank"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "live-rerank-schema", "--repo", repository,
		"--object", "schema/runbook.live-rerank", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	for _, item := range []struct{ id, body string }{
		{"runbook/deployment", "Support procedure for a Kubernetes deployment rollout"},
		{"runbook/refund", "Support procedure: inspect payment gateway timeout logs and refund idempotency status before retrying"},
		{"runbook/visitor", "Support procedure for office visitor registration"},
	} {
		body(t, kc(home, "put", "--command-id", "live-rerank-"+strings.TrimPrefix(item.id, "runbook/"), "--repo", repository,
			"--object", item.id, "--schema-ref", "schema/runbook.live-rerank", "--value", `{"body":`+string(mustJSON(t, item.body))+`,"secret":"must-not-leak"}`))
	}
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--action", "knowledge.read,knowledge.search,knowledge.rerank", "--repo", repository))
	syncIndexes(t, home, repository)
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Reranker: provider})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status, response := semanticHTTPAs(t, server, "/knowledge/v1/search:rerank", "agent:rerank-test", map[string]any{
		"catalog": catalog, "workspace": workspace, "query": "support procedure", "limit": 3,
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:live-http", "revision": 1, "operator": "SEMANTIC_RERANK",
			"criterion":            "Rank by usefulness for diagnosing a customer refund request that times out. Prefer directly actionable diagnosis.",
			"evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract":       map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d response=%#v", status, response)
	}
	rerankStage := asMap(t, response["rerank"])
	groups := rerankStage["groups"].([]any)
	first := asMap(t, asMap(t, groups[0])["refs"].([]any)[0])
	if first["object"] != "runbook/refund" {
		t.Fatalf("unexpected live HTTP ranking: %#v", rerankStage)
	}
	evidence := asMap(t, rerankStage["evidence"])
	if evidence["provider"] != "llm-native" || evidence["model"] != "gpt-5.6-luna" || evidence["candidateCount"] != float64(3) {
		t.Fatalf("live HTTP evidence = %#v", evidence)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestHTTPRerankReadsAuthorizedCanonicalCandidatesAndProjectsModelFields(t *testing.T) {
	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repository := "kr://acme/public/core"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "rerank-p1", "--repo", repository,
		"--object", "runbook/p1", "--value", `{"body":"deployment checklist","secret":"one"}`))
	body(t, kc(home, "put", "--command-id", "rerank-p2", "--repo", repository,
		"--object", "runbook/p2", "--value", `{"body":"refund timeout diagnosis","secret":"two"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--action", "knowledge.read", "--repo", repository))
	body(t, kc(home, "allow", "--principal", "agent:partial-rerank", "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:partial-rerank", "--action", "knowledge.read", "--repo", repository, "--object", "runbook/p1"))

	provider := &recordingReranker{}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Reranker: provider})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	status, response := rerankHTTP(t, server, map[string]any{
		"workspace": workspace,
		"candidates": []any{
			map[string]any{"repository": repository, "object": "runbook/p1"},
			map[string]any{"repository": repository, "object": "runbook/p2"},
		},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:runbook", "revision": 1,
			"operator": "SEMANTIC_RERANK", "criterion": "refund incident relevance",
			"evaluationProjection": map[string]any{"fields": []any{"body"}},
			"outputContract":       map[string]any{"topK": 1, "allowTies": false, "allowUnjudged": false},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d response=%#v", status, response)
	}
	if provider.calls != 1 || len(provider.request.Candidates) != 2 {
		t.Fatalf("provider calls/request = %d %#v", provider.calls, provider.request)
	}
	for _, candidate := range provider.request.Candidates {
		value := asMap(t, candidate.Value)
		if _, leaked := value["secret"]; leaked || len(value) != 1 {
			t.Fatalf("model-visible value leaked fields: %#v", value)
		}
	}
	groups := response["groups"].([]any)
	firstRef := asMap(t, asMap(t, groups[0])["refs"].([]any)[0])
	if firstRef["object"] != "runbook/p2" || response["complete"] != true {
		t.Fatalf("rerank response = %#v", response)
	}
	if len(response["notSelected"].([]any)) != 1 || response["searchView"] == nil {
		t.Fatalf("selection/basis = %#v", response)
	}
	evidence := asMap(t, response["evidence"])
	if evidence["provider"] != "llm-runtime" || evidence["candidateDigest"] == "" || evidence["candidateCount"] != float64(2) {
		t.Fatalf("evidence = %#v", evidence)
	}

	status, denied := rerankHTTPAs(t, server, "agent:partial-rerank", map[string]any{
		"workspace": workspace,
		"candidates": []any{
			map[string]any{"repository": repository, "object": "runbook/p1"},
			map[string]any{"repository": repository, "object": "runbook/p2"},
		},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:runbook", "revision": 1,
			"operator": "SEMANTIC_RERANK", "criterion": "relevance", "outputContract": map[string]any{},
		},
	})
	if status != http.StatusForbidden || asMap(t, denied["error"])["code"] != "FORBIDDEN" || provider.calls != 1 {
		t.Fatalf("unauthorized candidate reached provider: status=%d response=%#v calls=%d", status, denied, provider.calls)
	}
}

func TestHTTPRerankFailsClosedWithoutProviderBeforeCandidateRead(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--cmd", "read-workspace",
		"--catalog", "kr://acme/catalog", "--workspace", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:rerank-test", "--action", "knowledge.rerank",
		"--catalog", "kr://acme/catalog", "--workspace", "agent"))
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status, response := rerankHTTP(t, server, map[string]any{
		"workspace":  "agent",
		"candidates": []any{map[string]any{"repository": "kr://acme/public/core", "object": "runbook/p1"}},
		"spec": map[string]any{
			"specRef": "urn:semantic-spec:runbook", "revision": 1,
			"operator": "SEMANTIC_RERANK", "criterion": "relevance", "outputContract": map[string]any{},
		},
	})
	if status == http.StatusOK || asMap(t, response["error"])["code"] != "CAPABILITY_UNSATISFIED" {
		t.Fatalf("status=%d response=%#v", status, response)
	}
}
