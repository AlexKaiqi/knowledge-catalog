package retrieval

import "kc/kernel"

// SearchExpr is the smallest compositional search predicate. Exactly one of
// Clause, All, or Any must be present. SORT is request metadata and is never a
// predicate leaf.
//
// Not is deliberately absent: a provider must not claim it can enumerate the
// complement of a set unless the physical plan has proved a bounded universe.
type SearchExpr struct {
	Clause *SearchClause `json:"clause,omitempty"`
	All    []SearchExpr  `json:"all,omitempty"`
	Any    []SearchExpr  `json:"any,omitempty"`
}

const (
	MaxSearchExpressionDepth  = 16
	MaxSearchExpressionLeaves = 256
)

func SearchLeaf(clause SearchClause) SearchExpr {
	copy := clause
	return SearchExpr{Clause: &copy}
}

func SearchAll(children ...SearchExpr) SearchExpr {
	return SearchExpr{All: append([]SearchExpr{}, children...)}
}

func SearchAny(children ...SearchExpr) SearchExpr {
	return SearchExpr{Any: append([]SearchExpr{}, children...)}
}

func SearchWhere(expression SearchExpr) SearchRequest {
	copy := expression
	return SearchRequest{Expression: &copy}
}

// SearchPredicate returns the effective predicate. Legacy Clauses are
// interpreted as an implicit All after removing their optional SORT clause.
func SearchPredicate(req SearchRequest) (SearchExpr, bool) {
	if req.Expression != nil {
		return *req.Expression, true
	}
	leaves := make([]SearchExpr, 0, len(req.Clauses))
	for _, clause := range req.Clauses {
		if clause.Op != OpSort {
			leaves = append(leaves, SearchLeaf(clause))
		}
	}
	switch len(leaves) {
	case 0:
		return SearchExpr{}, false
	case 1:
		return leaves[0], true
	default:
		return SearchAll(leaves...), true
	}
}

// SearchSortClause returns the request-level ordering. New expression requests
// use SearchRequest.Sort; legacy requests may still carry SORT in Clauses.
func SearchSortClause(req SearchRequest) (SearchClause, bool) {
	if req.Sort != nil {
		return *req.Sort, true
	}
	for _, clause := range req.Clauses {
		if clause.Op == OpSort {
			return clause, true
		}
	}
	return SearchClause{}, false
}

// SearchClauses flattens predicate leaves in deterministic pre-order and then
// appends the optional SORT. It is intended for capability probing and field
// dependency discovery, not predicate evaluation.
func SearchClauses(req SearchRequest) []SearchClause {
	out := []SearchClause{}
	if expression, ok := SearchPredicate(req); ok {
		walkSearchExpression(expression, func(clause SearchClause) {
			out = append(out, clause)
		})
	}
	if sort, ok := SearchSortClause(req); ok {
		out = append(out, sort)
	}
	return out
}

func SearchHasOp(req SearchRequest, op SearchOp) bool {
	for _, clause := range SearchClauses(req) {
		if clause.Op == op {
			return true
		}
	}
	return false
}

func walkSearchExpression(expression SearchExpr, visit func(SearchClause)) {
	if expression.Clause != nil {
		visit(*expression.Clause)
		return
	}
	children := expression.All
	if expression.Any != nil {
		children = expression.Any
	}
	for _, child := range children {
		walkSearchExpression(child, visit)
	}
}

func validateSearchExpression(expression SearchExpr, depth int, leaves *int) error {
	if depth > MaxSearchExpressionDepth {
		return kernel.Fail(kernel.ErrUsageInvalid, "search expression exceeds maximum depth %d", MaxSearchExpressionDepth)
	}
	variants := 0
	if expression.Clause != nil {
		variants++
	}
	if expression.All != nil {
		variants++
	}
	if expression.Any != nil {
		variants++
	}
	if variants != 1 {
		return kernel.Fail(kernel.ErrUsageInvalid, "search expression requires exactly one of clause, all, or any")
	}
	if expression.Clause != nil {
		if expression.Clause.Op == OpSort {
			return kernel.Fail(kernel.ErrUsageInvalid, "SORT is request ordering and cannot appear inside search expression")
		}
		if err := validateClause(*expression.Clause); err != nil {
			return err
		}
		(*leaves)++
		if *leaves > MaxSearchExpressionLeaves {
			return kernel.Fail(kernel.ErrUsageInvalid, "search expression exceeds maximum leaves %d", MaxSearchExpressionLeaves)
		}
		return nil
	}
	children := expression.All
	operator := "all"
	if expression.Any != nil {
		children = expression.Any
		operator = "any"
	}
	if len(children) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "search expression %s requires at least one child", operator)
	}
	for _, child := range children {
		if err := validateSearchExpression(child, depth+1, leaves); err != nil {
			return err
		}
	}
	return nil
}

func resolveSearchExpression(expression SearchExpr, spec AccessSpec) (SearchExpr, error) {
	if expression.Clause != nil {
		resolved, err := ResolveSearchClause(*expression.Clause, spec)
		if err != nil {
			return SearchExpr{}, err
		}
		return SearchLeaf(resolved), nil
	}
	if expression.All != nil {
		children := make([]SearchExpr, len(expression.All))
		for i, child := range expression.All {
			resolved, err := resolveSearchExpression(child, spec)
			if err != nil {
				return SearchExpr{}, err
			}
			children[i] = resolved
		}
		return SearchAll(children...), nil
	}
	children := make([]SearchExpr, len(expression.Any))
	for i, child := range expression.Any {
		resolved, err := resolveSearchExpression(child, spec)
		if err != nil {
			return SearchExpr{}, err
		}
		children[i] = resolved
	}
	return SearchAny(children...), nil
}
