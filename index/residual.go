package index

import (
	"kc/retrieval"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kc/knowledge"
)

var residualTokenUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_\p{L}]+`)

// matchesResidual evaluates the logical predicate against hydrated Canonical
// values when a provider only guarantees a candidate superset.
func matchesResidual(repo knowledge.Repository, value knowledge.KnowledgeValue, observations []knowledge.UnitObservation, req retrieval.SearchRequest, spec retrieval.AccessSpec) (bool, error) {
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
	return matchesResidualExpression(expression, doc, eligible, spec)
}

func matchesResidualExpression(expression retrieval.SearchExpr, doc CompiledDoc, eligible map[string]struct{}, spec retrieval.AccessSpec) (bool, error) {
	if expression.Clause == nil {
		children := expression.All
		isAny := false
		if expression.Any != nil {
			children = expression.Any
			isAny = true
		}
		for _, child := range children {
			matched, err := matchesResidualExpression(child, doc, eligible, spec)
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
		return residualMatch(doc.Text, clause.Value, clause.Mode), nil
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
			if residualMatch(item, clause.Value, clause.Mode) {
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
		return containsScalar(values, clause.Value)
	case retrieval.OpIN:
		for _, target := range clause.Values {
			if containsScalar(values, target) {
				return true
			}
		}
		return false
	case retrieval.OpNEQ:
		return len(values) > 0 && !containsScalar(values, clause.Value)
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

func containsScalar(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func scalarCompare(left, right, fieldType string, op retrieval.SearchOp) bool {
	cmp := strings.Compare(left, right)
	if retrieval.NumericType(fieldType) {
		l, lerr := strconv.ParseFloat(left, 64)
		r, rerr := strconv.ParseFloat(right, 64)
		if lerr != nil || rerr != nil {
			return false
		}
		switch {
		case l < r:
			cmp = -1
		case l > r:
			cmp = 1
		default:
			cmp = 0
		}
	}
	if retrieval.TemporalType(fieldType) {
		layout := time.RFC3339
		if strings.EqualFold(strings.TrimSpace(fieldType), "date") {
			layout = "2006-01-02"
		}
		l, lerr := time.Parse(layout, left)
		r, rerr := time.Parse(layout, right)
		if lerr != nil || rerr != nil {
			return false
		}
		switch {
		case l.Before(r):
			cmp = -1
		case l.After(r):
			cmp = 1
		default:
			cmp = 0
		}
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

func residualMatch(text, query string, mode retrieval.MatchMode) bool {
	text = strings.ToLower(text)
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(query)) {
		term = residualTokenUnsafe.ReplaceAllString(term, "")
		if term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return false
	}
	if mode == retrieval.MatchPhrase {
		return strings.Contains(text, strings.Join(terms, " "))
	}
	for _, term := range terms {
		found := strings.Contains(text, term)
		if mode == retrieval.MatchAnyTerms && found {
			return true
		}
		if mode != retrieval.MatchAnyTerms && !found {
			return false
		}
	}
	return mode != retrieval.MatchAnyTerms
}
