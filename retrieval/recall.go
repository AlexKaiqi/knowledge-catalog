package retrieval

import "kc/kernel"

// RecallStrategy selects how the fixed-scope candidate window is formed. It
// is a request-level closed set, not a query DSL: lexical is the incumbent
// default, semantic runs the k-NN window over the derived vector projection,
// and hybrid stays reserved until its cross-lane fusion contract is selected
// (docs/reviewed/retrieval.md, ADR-028). Omitting the field keeps every existing
// request and contract on lexical.
type RecallStrategy string

const (
	RecallLexical  RecallStrategy = "lexical"
	RecallSemantic RecallStrategy = "semantic"
	RecallHybrid   RecallStrategy = "hybrid"
)

// ValidRecall reports whether the strategy name is part of the closed set.
func ValidRecall(recall RecallStrategy) bool {
	switch recall {
	case "", RecallLexical, RecallSemantic, RecallHybrid:
		return true
	default:
		return false
	}
}

// validateRecall applies the request-level rules. Semantic recall needs a
// text query to embed; pure filter requests cannot form a vector window and
// must not silently fall back to lexical. Hybrid keeps its reserved status:
// the name exists so clients can be rejected deterministically before any
// fusion behavior is improvised.
// ValidateRecall applies the recall-strategy rules on their own so transports
// can reject a bad strategy before the rest of the request is assembled; the
// full ValidateSearch includes these checks as well.
func ValidateRecall(req SearchRequest) error {
	return validateRecall(req)
}

func validateRecall(req SearchRequest) error {
	if !ValidRecall(req.Recall) {
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown recall strategy %q", string(req.Recall))
	}
	if req.Recall == RecallHybrid {
		return kernel.Fail(kernel.ErrUsageInvalid, "hybrid recall is reserved until its fusion contract is selected")
	}
	if req.Recall == RecallSemantic && !recallableText(req) {
		return kernel.Fail(kernel.ErrUsageInvalid, "semantic recall requires a MATCH text query")
	}
	return nil
}

// recallableText reports whether the request carries at least one MATCH text
// leaf. Legacy clauses and typed expressions must agree on this shape.
func recallableText(req SearchRequest) bool {
	if req.Expression != nil {
		return expressionHasMatch(*req.Expression)
	}
	for _, clause := range req.Clauses {
		if clause.Op == OpMatch {
			return true
		}
	}
	return false
}

func expressionHasMatch(expr SearchExpr) bool {
	if expr.Clause != nil {
		return expr.Clause.Op == OpMatch
	}
	for _, child := range expr.All {
		if expressionHasMatch(child) {
			return true
		}
	}
	for _, child := range expr.Any {
		if expressionHasMatch(child) {
			return true
		}
	}
	return false
}
