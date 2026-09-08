package index

import (
	"kc/kernel"
	"kc/retrieval"
	"strings"

	"kc/knowledge"
)

// matchesResidual evaluates the logical predicate against hydrated Canonical
// values when a provider only guarantees a candidate superset.
func matchesResidual(repo knowledge.Repository, value knowledge.KnowledgeValue, observations []knowledge.UnitObservation, req retrieval.SearchRequest, spec retrieval.AccessSpec, verifier ...ResidualMatchVerifier) (bool, error) {
	doc, err := compileProjectionDocumentObserved(repo, value, observations, spec)
	if err != nil {
		return false, err
	}
	eligible := map[string]struct{}{}
	for _, field := range doc.EligibleFields {
		eligible[field] = struct{}{}
	}
	expression, ok := retrieval.SearchPredicate(req)
	if !ok {
		return false, nil
	}
	return matchesResidualExpression(expression, doc, eligible, spec, verifier...)
}

func matchesResidualExpression(expression retrieval.SearchExpr, doc CompiledDoc, eligible map[string]struct{}, spec retrieval.AccessSpec, verifier ...ResidualMatchVerifier) (bool, error) {
	if expression.Clause == nil {
		children := expression.All
		isAny := false
		if expression.Any != nil {
			children = expression.Any
			isAny = true
		}
		for _, child := range children {
			matched, err := matchesResidualExpression(child, doc, eligible, spec, verifier...)
			if err != nil {
				return false, err
			}
			if isAny && matched {
				return true, nil
			}
			if !isAny && !matched {
				return false, nil
			}
		}
		return !isAny, nil
	}
	clause := *expression.Clause
	if clause.Op == retrieval.OpMatch && clause.Field == nil && clause.Path == "" {
		return residualMatch(doc.Text, clause.Value, clause.Mode, verifier...)
	}
	field, err := spec.ResolveField(*clause.Field)
	if err != nil {
		return false, err
	}
	if _, applies := eligible[field.FieldRef.Key()]; !applies {
		return false, nil
	}
	values := []string{}
	textValues := []string{}
	for _, cell := range doc.Cells {
		if cell.Field != field.FieldRef.Key() {
			continue
		}
		values = append(values, cell.Value)
		if cell.TextValue != "" {
			textValues = append(textValues, cell.TextValue)
		}
	}
	switch clause.Op {
	case retrieval.OpMatch:
		for _, item := range textValues {
			matched, err := residualMatch(item, clause.Value, clause.Mode, verifier...)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	case retrieval.OpExists:
		return len(values) > 0, nil
	case retrieval.OpMissing:
		return len(values) == 0, nil
	case retrieval.OpEQ, retrieval.OpIN, retrieval.OpNEQ, retrieval.OpPrefix, retrieval.OpContains, retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE:
		return residualScalarClause(clause, field.Type, values), nil
	}
	return false, nil
}

func residualScalarClause(clause retrieval.SearchClause, fieldType string, values []string) bool {
	switch clause.Op {
	case retrieval.OpEQ:
		return containsScalar(fieldType, values, clause.Value)
	case retrieval.OpIN:
		for _, target := range clause.Values {
			if containsScalar(fieldType, values, target) {
				return true
			}
		}
		return false
	case retrieval.OpNEQ:
		return len(values) > 0 && !containsScalar(fieldType, values, clause.Value)
	case retrieval.OpPrefix:
		for _, value := range values {
			if strings.HasPrefix(value, clause.Value) {
				return true
			}
		}
		return false
	case retrieval.OpContains:
		for _, value := range values {
			if strings.Contains(value, clause.Value) {
				return true
			}
		}
		return false
	case retrieval.OpGT, retrieval.OpGTE, retrieval.OpLT, retrieval.OpLTE:
		for _, value := range values {
			if scalarCompare(value, clause.Value, fieldType, clause.Op) {
				return true
			}
		}
	}
	return false
}

func containsScalar(fieldType string, values []string, target string) bool {
	for _, value := range values {
		if cmp, err := retrieval.CompareScalarValues(fieldType, value, target); err == nil && cmp == 0 {
			return true
		}
	}
	return false
}

func scalarCompare(left, right, fieldType string, op retrieval.SearchOp) bool {
	cmp, err := retrieval.CompareScalarValues(fieldType, left, right)
	if err != nil {
		return false
	}
	switch op {
	case retrieval.OpGT:
		return cmp > 0
	case retrieval.OpGTE:
		return cmp >= 0
	case retrieval.OpLT:
		return cmp < 0
	case retrieval.OpLTE:
		return cmp <= 0
	default:
		return false
	}
}

func residualMatch(text, query string, mode retrieval.MatchMode, verifiers ...ResidualMatchVerifier) (bool, error) {
	if len(verifiers) == 0 || verifiers[0] == nil {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "analyzed MATCH residual requires a provider verifier")
	}
	return verifiers[0].VerifyResidualMatch(text, query, mode)
}
