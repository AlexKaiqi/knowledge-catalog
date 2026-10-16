package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
)

type analyzedToken struct {
	Term           string `json:"token"`
	Position       *int   `json:"position"`
	PositionLength int    `json:"positionLength"`
}

func (e *openSearchEngine) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	return e.VerifyResidualMatchContext(context.Background(), text, query, mode)
}

// VerifyResidualMatchContext checks new observations with the same analyzer as
// the published text fields. Analysis is read-only; it never indexes a value.
func (e *openSearchEngine) VerifyResidualMatchContext(ctx context.Context, text, query string, mode retrieval.MatchMode) (bool, error) {
	if mode != "" && mode != retrieval.MatchAllTerms && mode != retrieval.MatchAnyTerms && mode != retrieval.MatchPhrase {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unknown residual MATCH mode")
	}
	control, _, err := e.loadControlContext(ctx)
	if err != nil {
		return false, err
	}
	if control.State != index.ProjectionStateReady || !strings.HasPrefix(control.ActiveIndex, e.prefix+"-g-") {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "residual MATCH requires the owned published projection")
	}
	wanted, err := e.analyzeResidual(ctx, control.ActiveIndex, query)
	if err != nil || len(wanted) == 0 {
		return false, err
	}
	actual, err := e.analyzeResidual(ctx, control.ActiveIndex, text)
	if err != nil {
		return false, err
	}
	positions := map[string]map[int]bool{}
	for _, token := range actual {
		if positions[token.Term] == nil {
			positions[token.Term] = map[int]bool{}
		}
		positions[token.Term][*token.Position] = true
	}
	if mode == retrieval.MatchPhrase {
		return analyzedPhraseMatches(positions, wanted), nil
	}
	for _, token := range wanted {
		found := len(positions[token.Term]) > 0
		if mode == retrieval.MatchAnyTerms && found {
			return true, nil
		}
		if mode != retrieval.MatchAnyTerms && !found {
			return false, nil
		}
	}
	return mode != retrieval.MatchAnyTerms, nil
}

func (e *openSearchEngine) analyzeResidual(ctx context.Context, generation, text string) ([]analyzedToken, error) {
	status, body, err := e.doContext(ctx, http.MethodPost, "/"+url.PathEscape(generation)+"/_analyze", map[string]any{"field": "all_text", "text": text})
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "OpenSearch residual analysis returned HTTP %d", status)
	}
	var response struct {
		Tokens *[]analyzedToken `json:"tokens"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Tokens == nil {
		return nil, fmt.Errorf("OpenSearch residual analysis has no valid tokens")
	}
	previous := -1
	for _, token := range *response.Tokens {
		if token.Term == "" || token.Position == nil || *token.Position <= previous || token.PositionLength > 1 {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "OpenSearch residual analysis cannot prove linear token positions")
		}
		previous = *token.Position
	}
	return *response.Tokens, nil
}

func analyzedPhraseMatches(positions map[string]map[int]bool, wanted []analyzedToken) bool {
	for start := range positions[wanted[0].Term] {
		matched := true
		for _, token := range wanted {
			at := start + *token.Position - *wanted[0].Position
			if !positions[token.Term][at] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
