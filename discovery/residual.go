package discovery

import (
	"kc/kernel"
	"strings"

	objectmodel "kc/object"
	"kc/object/reader"
)

// matchesResidual evaluates the logical predicate against hydrated Canonical
// values when a provider only guarantees a candidate superset.
func MatchesResidual(repo objectmodel.Repository, value objectmodel.ObjectValue, observations []objectmodel.UnitObservation, req SearchRequest, spec AccessSpec, verifier ...ResidualMatchVerifier) (bool, error) {
	doc, err := ExtractFields(repo, value, observations, spec)
	if err != nil {
		return false, err
	}
	return MatchFields(req, doc, spec, verifier...)
}

// MatchFields evaluates a request against already extracted logical fields.
func MatchFields(req SearchRequest, doc FieldSet, spec AccessSpec, verifier ...ResidualMatchVerifier) (bool, error) {
	eligible := map[string]struct{}{}
	for _, field := range doc.EligibleFields {
		eligible[field] = struct{}{}
	}
	expression, ok := SearchPredicate(req)
	if !ok {
		return false, nil
	}
	return matchesResidualExpression(expression, doc, eligible, spec, verifier...)
}

func matchesResidualExpression(expression SearchExpr, doc FieldSet, eligible map[string]struct{}, spec AccessSpec, verifier ...ResidualMatchVerifier) (bool, error) {
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
	if clause.Op == OpMatch && clause.Field == nil && clause.Path == "" {
		return residualMatch(doc.Text, clause.Value, clause.Mode, verifier...)
	}
	field, err := spec.ResolveField(*clause.Field)
	if err != nil {
		return false, err
	}
	if clause.Op != OpMatch {
		if _, applies := eligible[field.FieldRef.Key()]; !applies {
			return false, nil
		}
	}
	values := []string{}
	textValues := []string{}
	for _, cell := range doc.Values {
		if cell.Field.FieldRef.Key() != field.FieldRef.Key() {
			continue
		}
		for _, hint := range cell.Field.Access {
			if hint == reader.HintFilter {
				values = append(values, cell.Value)
				break
			}
		}
		if cell.Field.Has(reader.HintText) {
			textValues = append(textValues, cell.Value)
		}
	}
	switch clause.Op {
	case OpMatch:
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
	case OpExists:
		return len(values) > 0, nil
	case OpMissing:
		return len(values) == 0, nil
	case OpEQ, OpIN, OpNEQ, OpPrefix, OpContains, OpGT, OpGTE, OpLT, OpLTE:
		return residualScalarClause(clause, field.Type, values), nil
	}
	return false, nil
}

func residualScalarClause(clause SearchClause, fieldType string, values []string) bool {
	switch clause.Op {
	case OpEQ:
		return containsScalar(fieldType, values, clause.Value)
	case OpIN:
		for _, target := range clause.Values {
			if containsScalar(fieldType, values, target) {
				return true
			}
		}
		return false
	case OpNEQ:
		return len(values) > 0 && !containsScalar(fieldType, values, clause.Value)
	case OpPrefix:
		for _, value := range values {
			if strings.HasPrefix(value, clause.Value) {
				return true
			}
		}
		return false
	case OpContains:
		for _, value := range values {
			if strings.Contains(value, clause.Value) {
				return true
			}
		}
		return false
	case OpGT, OpGTE, OpLT, OpLTE:
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
		if cmp, err := CompareScalarValues(fieldType, value, target); err == nil && cmp == 0 {
			return true
		}
	}
	return false
}

func scalarCompare(left, right, fieldType string, op SearchOp) bool {
	cmp, err := CompareScalarValues(fieldType, left, right)
	if err != nil {
		return false
	}
	switch op {
	case OpGT:
		return cmp > 0
	case OpGTE:
		return cmp >= 0
	case OpLT:
		return cmp < 0
	case OpLTE:
		return cmp <= 0
	default:
		return false
	}
}

func residualMatch(text, query string, mode MatchMode, verifiers ...ResidualMatchVerifier) (bool, error) {
	if len(verifiers) == 0 || verifiers[0] == nil {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "analyzed MATCH residual requires a provider verifier")
	}
	return verifiers[0].VerifyResidualMatch(text, query, mode)
}

// ResidualMatchVerifier supplies the same analyzed term/phrase semantics as
// the provider's MATCH implementation. Without it, a Superset plan containing
// MATCH cannot be verified and must fail before candidate retrieval.
type ResidualMatchVerifier interface {
	VerifyResidualMatch(text, query string, mode MatchMode) (bool, error)
}
