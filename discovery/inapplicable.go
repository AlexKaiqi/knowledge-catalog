package discovery

import "kc/object/reader"

// PruneInapplicable removes predicates that only a selected declaration whose
// input Schema is outside this scope could answer. No object in scope can
// carry such a field, so the predicate is a complete miss, not an undeclared
// or unsupported condition (discovery 完整性). empty reports that the whole
// predicate cannot match; the caller returns a complete zero-hit result. The
// other conditions are still checked then, so a typo is never hidden by an
// empty scope. Ordering is left as is: SORT never narrows the matching set.
func PruneInapplicable(req SearchRequest, spec AccessSpec) (SearchRequest, bool, error) {
	if len(spec.inapplicable) == 0 {
		return req, false, nil
	}
	out := req
	empty := false
	if req.Expression != nil {
		pruned, none := spec.pruneInapplicableExpr(*req.Expression)
		empty = none
		out.Expression = &pruned
	} else {
		out.Clauses = nil
		for _, clause := range req.Clauses {
			if clause.Op != OpSort && spec.inapplicableClause(clause) {
				empty = true
				continue
			}
			out.Clauses = append(out.Clauses, clause)
		}
	}
	if !empty {
		return out, false, nil
	}
	for _, clause := range SearchClauses(req) {
		if clause.Op == OpSort || !spec.inapplicableClause(clause) {
			if _, err := ResolveSearchClause(clause, spec); err != nil {
				return req, false, err
			}
		}
	}
	return req, true, nil
}

func (s AccessSpec) pruneInapplicableExpr(expression SearchExpr) (SearchExpr, bool) {
	if expression.Clause != nil {
		return expression, s.inapplicableClause(*expression.Clause)
	}
	if expression.Any != nil {
		kept := []SearchExpr{}
		for _, child := range expression.Any {
			if pruned, empty := s.pruneInapplicableExpr(child); !empty {
				kept = append(kept, pruned)
			}
		}
		return SearchExpr{Any: kept}, len(kept) == 0
	}
	kept := make([]SearchExpr, 0, len(expression.All))
	for _, child := range expression.All {
		pruned, empty := s.pruneInapplicableExpr(child)
		if empty {
			return expression, true
		}
		kept = append(kept, pruned)
	}
	return SearchExpr{All: kept}, false
}

// inapplicableClause reports a predicate that no ready field in scope serves
// while a selected inapplicable member declares it for this operation.
func (s AccessSpec) inapplicableClause(clause SearchClause) bool {
	unfielded := clause.Op == OpMatch && clause.Path == "" && clause.Field == nil
	ref := FieldRef{Path: clause.Path}
	if clause.Field != nil {
		ref = *clause.Field
	}
	for _, field := range s.Fields {
		if unfielded && field.Has(reader.HintText) {
			return false
		}
		if !unfielded && field.Path == ref.Path && (ref.Schema == "" || field.Schema == ref.Schema) && (ref.Aspect == "" || field.Aspect == ref.Aspect) {
			return false
		}
	}
	for _, build := range s.inapplicable {
		if !declarationAllows(build, clause.Op) {
			continue
		}
		if unfielded {
			return true
		}
		for _, input := range build.inputs {
			if input.Field.Path == ref.Path && (ref.Schema == "" || input.Field.Schema == ref.Schema) {
				return true
			}
		}
	}
	return false
}
