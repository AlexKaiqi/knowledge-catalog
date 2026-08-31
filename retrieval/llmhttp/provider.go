package llmhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

const (
	defaultTimeout = 45 * time.Second
	maxErrorBytes  = 1024
	promptRevision = "llmhttp-listwise-v1"
)

// Config is an explicit Responses-compatible listwise reranker configuration.
// APIKey is process-only configuration and is never copied into requests,
// results, evidence or diagnostics.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Provider struct {
	endpoint string
	apiKey   string
	model    string
	timeout  time.Duration
	client   *http.Client
}

func New(config Config) (*Provider, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	apiKey := strings.TrimSpace(config.APIKey)
	model := strings.TrimSpace(config.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "LLM reranker requires base URL, API key and model")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "LLM reranker base URL must be HTTP(S)")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/responses"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{endpoint: parsed.String(), apiKey: apiKey, model: model, timeout: timeout, client: client}, nil
}

type responseRequest struct {
	Model     string            `json:"model"`
	Input     []responseMessage `json:"input"`
	Reasoning reasoningConfig   `json:"reasoning"`
	Text      textConfig        `json:"text"`
}

type responseMessage struct {
	Role    string            `json:"role"`
	Content []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

type textConfig struct {
	Format jsonSchemaFormat `json:"format"`
}

type jsonSchemaFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type modelCandidate struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
}

type modelInput struct {
	Criterion  string           `json:"criterion"`
	Candidates []modelCandidate `json:"candidates"`
}

type responseEnvelope struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type modelRanking struct {
	Groups []struct {
		CandidateIDs []string `json:"candidateIds"`
	} `json:"groups"`
	Unjudged []string `json:"unjudged"`
}

// Rerank sends the complete candidate window in one listwise Responses call.
// It never batches or retries because either would change ranking semantics or
// risk charging the same non-idempotent model request twice.
func (p *Provider) Rerank(ctx context.Context, request retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
	if p == nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LLM reranker is not configured")
	}
	ids := make(map[string]knowledge.KnowledgeRef, len(request.Candidates))
	candidates := make([]modelCandidate, len(request.Candidates))
	for i, candidate := range request.Candidates {
		id := fmt.Sprintf("candidate_%d", i+1)
		ids[id] = candidate.Ref
		candidates[i] = modelCandidate{ID: id, Value: candidate.Value}
	}
	input, err := json.Marshal(modelInput{Criterion: request.Spec.Criterion, Candidates: candidates})
	if err != nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrUsageInvalid, "LLM rerank input is not JSON serializable")
	}
	payload := responseRequest{
		Model: p.model,
		Input: []responseMessage{
			{Role: "system", Content: []responseContent{{Type: "input_text", Text: "Rank every candidate by the supplied criterion using only candidate value. Return one ordered list of tie groups. Put a candidate in unjudged only when its visible value is insufficient. Never invent, omit, or duplicate candidate IDs."}}},
			{Role: "user", Content: []responseContent{{Type: "input_text", Text: string(input)}}},
		},
		Reasoning: reasoningConfig{Effort: "none"},
		Text:      textConfig{Format: rankingSchema()},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrUsageInvalid, "LLM rerank request is not JSON serializable")
	}
	requestContext, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "cannot construct LLM rerank request")
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(requestContext.Err(), context.DeadlineExceeded) {
			return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "LLM reranker timed out or was canceled")
		}
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "LLM reranker request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBytes))
		switch {
		case response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusRequestTimeout || response.StatusCode >= 500:
			return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "LLM reranker returned HTTP %d", response.StatusCode)
		default:
			return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LLM reranker rejected the request with HTTP %d", response.StatusCode)
		}
	}
	var envelope responseEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "LLM reranker returned invalid response JSON")
	}
	if envelope.Status != "" && envelope.Status != "completed" {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "LLM reranker did not complete the structured response")
	}
	outputText := ""
	for _, item := range envelope.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				outputText = content.Text
				break
			}
		}
	}
	if outputText == "" {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "LLM reranker returned no structured output")
	}
	var ranking modelRanking
	if err := json.Unmarshal([]byte(outputText), &ranking); err != nil {
		return retrieval.RerankProviderResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "LLM reranker returned invalid structured output")
	}
	result := retrieval.RerankProviderResult{
		Provider: "llm-native", Model: p.model, ModelRevision: strings.TrimSpace(envelope.Model), PromptRevision: promptRevision,
	}
	for i, group := range ranking.Groups {
		refs, err := resolveCandidateIDs(ids, group.CandidateIDs)
		if err != nil {
			return retrieval.RerankProviderResult{}, err
		}
		result.Groups = append(result.Groups, retrieval.RankGroup{Rank: i + 1, Refs: refs})
	}
	result.Unjudged, err = resolveCandidateIDs(ids, ranking.Unjudged)
	if err != nil {
		return retrieval.RerankProviderResult{}, err
	}
	return result, nil
}

func resolveCandidateIDs(known map[string]knowledge.KnowledgeRef, ids []string) ([]knowledge.KnowledgeRef, error) {
	refs := make([]knowledge.KnowledgeRef, len(ids))
	for i, id := range ids {
		ref, ok := known[id]
		if !ok {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "LLM reranker returned an unknown candidate id")
		}
		refs[i] = ref
	}
	return refs, nil
}

func rankingSchema() jsonSchemaFormat {
	return jsonSchemaFormat{
		Type: "json_schema", Name: "semantic_rerank", Strict: true,
		Schema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"groups": map[string]any{
					"type": "array", "items": map[string]any{
						"type": "object", "additionalProperties": false,
						"properties": map[string]any{"candidateIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}},
						"required":   []string{"candidateIds"},
					},
				},
				"unjudged": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"groups", "unjudged"},
		},
	}
}
