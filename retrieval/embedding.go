package retrieval

import (
	"context"
	"strings"

	"kc/kernel"
)

// EmbeddingRequest carries the recallable query texts for one semantic
// window. Documents are embedded at projection-build time by the projection
// control chain; this port serves the request-time query vector only
// (docs/RETRIEVAL.md §8.1).
type EmbeddingRequest struct {
	Texts []string
}

// EmbeddingResult returns one vector per request text, in request order.
// Provider and Model identify the model that produced the vectors; they must
// be echoed into result claims and evidence so a window can never be judged
// without its model identity.
type EmbeddingResult struct {
	Vectors    [][]float32
	Provider   string
	Model      string
	Dimensions int
}

// Embedder is the request-time query-vector port. Implementations own model
// selection and transport; they must not batch-split one request (a split
// changes neither semantics but must not silently retry a paid call), and
// they must return vectors in request order.
type Embedder interface {
	Embed(ctx context.Context, request EmbeddingRequest) (EmbeddingResult, error)
}

// SemanticQueryText extracts the recallable text of a validated semantic
// request: MATCH leaf values in request order, joined with single spaces.
// Pure filter leaves contribute nothing; a request without any MATCH text is
// rejected by ValidateSearch before this helper runs.
func SemanticQueryText(request SearchRequest) string {
	values := make([]string, 0, 4)
	collectMatchValues(request, &values)
	return strings.TrimSpace(strings.Join(values, " "))
}

func collectMatchValues(request SearchRequest, values *[]string) {
	if request.Expression != nil {
		collectExpressionMatches(*request.Expression, values)
		return
	}
	for _, clause := range request.Clauses {
		if clause.Op == OpMatch {
			*values = append(*values, clause.Value)
		}
	}
}

func collectExpressionMatches(expr SearchExpr, values *[]string) {
	if expr.Clause != nil {
		if expr.Clause.Op == OpMatch {
			*values = append(*values, expr.Clause.Value)
		}
		return
	}
	for _, child := range expr.All {
		collectExpressionMatches(child, values)
	}
	for _, child := range expr.Any {
		collectExpressionMatches(child, values)
	}
}

// ApproximateSources lists the disclosure strings a semantic or hybrid result
// must carry: the approximate window, the vector index interval, and the
// embedding model identity. §8.1 forbids claiming completeness for these
// results, so the envelope is forced to partial even if a lane overclaimed.
func MarkApproximate(result *SearchResult, claims ...string) error {
	if result == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "approximate result envelope is missing")
	}
	result.Completeness = CompletenessPartial
	seen := map[string]bool{}
	for _, claim := range append([]string{"recall=semantic: approximate k-NN ranking window"}, claims...) {
		if claim == "" || seen[claim] {
			continue
		}
		seen[claim] = true
		result.Claims = append(result.Claims, claim)
	}
	return nil
}
